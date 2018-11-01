package adaptor

import (
	"fmt"
	"net"

	"github.com/chenchun/kube-bmlb/api"
	"github.com/chenchun/kube-bmlb/lvs"
	"github.com/chenchun/kube-bmlb/utils/dbus"
	"github.com/chenchun/kube-bmlb/utils/ipset"
	"github.com/chenchun/kube-bmlb/utils/iptables"
	"github.com/chenchun/kube-bmlb/utils/sysctl"
	"github.com/docker/libnetwork/ipvs"
	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	"k8s.io/utils/exec"
)

type LVSAdaptor struct {
	lvsHandler           lvs.Interface
	iptHandler           iptables.Interface
	ipsetHandler         ipset.Interface
	virtualServerAddress net.IP
}

func NewLVSAdaptor(virtualServerAddress net.IP) *LVSAdaptor {
	return &LVSAdaptor{
		lvsHandler:           lvs.New(),
		iptHandler:           iptables.New(exec.New(), dbus.New(), iptables.ProtocolIpv4),
		ipsetHandler:         ipset.New(exec.New()),
		virtualServerAddress: virtualServerAddress}
}

func (a *LVSAdaptor) checkSysctl() {
	if err := sysctl.EnsureSysctl("net/ipv4/vs/conntrack", 1); err != nil {
		glog.Warningf("failed to ensure net/ipv4/vs/conntrack: %v", err)
	}
}

func (a *LVSAdaptor) Build(lbSvcs []*v1.Service, endpoints []*v1.Endpoints) {
	a.checkSysctl()
	endpointsMap := map[string]map[string][]*v1.Endpoints{} // Namespace->Name->Endpoints
	// virtual server is like 10.0.0.2:8080, service has allocated ports in annotation
	// so build a map which maps ports to service
	portServiceMap := []map[int32]*v1.Service{{}, {}} //protocol port:service
	for i := range lbSvcs {
		svc := lbSvcs[i]
		if _, ok := endpointsMap[svc.Namespace]; !ok {
			endpointsMap[svc.Namespace] = map[string][]*v1.Endpoints{}
		}
		endpointsMap[svc.Namespace][svc.Name] = []*v1.Endpoints{}
		lbPorts := api.DecodeL4Ports(svc.Annotations[api.ANStatusBindedPort])
		for protol, ports := range lbPorts {
			for _, port := range ports {
				portServiceMap[protol][port] = svc
			}
		}
	}
	a.buildIptables(portServiceMap)
	for i := range endpoints {
		enp := endpoints[i]
		if _, exist := endpointsMap[enp.Namespace]; !exist {
			continue
		}
		nameMap := endpointsMap[enp.Namespace]
		if _, exist := nameMap[enp.Name]; !exist {
			continue
		}
		nameMap[enp.Name] = append(nameMap[enp.Name], enp)
	}
	vss, err := a.lvsHandler.GetVirtualServers()
	if err != nil {
		glog.Errorf("failed to get virtual servers: %v", err)
		return
	}
	// check existing virtual services
	for i := range vss {
		vs := vss[i]
		var index = 0
		if vs.Protocol == "UDP" {
			index = 1
		}
		if !vs.Address.Equal(a.virtualServerAddress) {
			//TODO should we delete this virtual server in case changing a.virtualServerAddress or just continue in order to not delete user customer lvs
			// lvs doesn't support comment
			if err := a.lvsHandler.DeleteVirtualServer(vs); err != nil {
				// raise a warning instead of error as we will retry later
				glog.Warningf("failed to delete virtual server %s: %v", vs.String(), err)
			}
			continue
		}
		if svc, ok := portServiceMap[index][int32(vs.Port)]; !ok {
			// service not exists, but virtual server exists
			if err := a.lvsHandler.DeleteVirtualServer(vs); err != nil {
				// raise a warning instead of error as we will retry later
				glog.Warningf("failed to delete virtual server %s: %v", vs.String(), err)
			}
		} else {
			delete(portServiceMap[index], int32(vs.Port))
			// syncing real servers
			edpts := endpointsMap[svc.Namespace][svc.Name]
			if len(edpts) == 0 {
				//  endpoints not exists for now
				continue
			}
			expectRSs := getExpectRS(edpts, vs, svc)
			rss, err := a.lvsHandler.GetRealServers(vs)
			if err != nil {
				glog.Warningf("failed to get real servers for virtual server %s: %v", vs.String(), err)
				continue
			}
			for j := range rss {
				rs := rss[j]
				rsStr := fmt.Sprintf("%s:%d", rs.Address.String(), rs.Port)
				if _, ok := expectRSs[rsStr]; !ok {
					if err := a.lvsHandler.DeleteRealServer(vs, rs); err != nil {
						glog.Warningf("failed to del real server %s: %v", rs.String(), err)
					}
				} else {
					delete(expectRSs, rsStr)
				}
			}
			// add new real servers
			for str := range expectRSs {
				expectRS := expectRSs[str]
				if err := a.lvsHandler.AddRealServer(vs, &expectRS); err != nil {
					glog.Warningf("failed to add real server %s: %v", expectRS.String(), err)
				}
			}
		}
	}

	// create not exist virtual services and real servers
	for i := 0; i < len(portServiceMap); i++ {
		protocol := "TCP"
		if i == 1 {
			protocol = "UDP"
		}
		for port, svc := range portServiceMap[i] {
			vs := &lvs.VirtualServer{Address: a.virtualServerAddress, Port: uint16(port), Protocol: protocol, Scheduler: ipvs.RoundRobin}
			if err := a.lvsHandler.AddVirtualServer(vs); err != nil {
				// raise a warning instead of error as we will retry later
				glog.Warningf("failed to add virtual server %s: %v", vs.String(), err)
				continue
			}
			edpts := endpointsMap[svc.Namespace][svc.Name]
			if len(edpts) == 0 {
				//  endpoints not exists for now
				continue
			}
			expectRSs := getExpectRS(edpts, vs, svc)
			for str := range expectRSs {
				expectRS := expectRSs[str]
				if err := a.lvsHandler.AddRealServer(vs, &expectRS); err != nil {
					glog.Warningf("failed to add real server %s: %v", expectRS.String(), err)
				}
			}
		}
	}
}

// getExpectRS returns {"10.0.0.2:8080": RealServer}
func getExpectRS(edpts []*v1.Endpoints, vs *lvs.VirtualServer, svc *v1.Service) map[string]lvs.RealServer {
	portIndex := -1
	for i, p := range svc.Spec.Ports {
		if uint16(p.Port) == vs.Port {
			portIndex = i
			break
		}
	}
	expectRS := map[string]lvs.RealServer{}
	if portIndex == -1 {
		// should never happen
		glog.Errorf("portIndex %d", portIndex)
		return expectRS
	}
	for _, edpt := range edpts {
		for _, subset := range edpt.Subsets {
			for _, addr := range subset.Addresses {
				if len(subset.Ports) != len(svc.Spec.Ports) {
					// endpoint is not synced with service yet
					continue
				}
				expectRS[fmt.Sprintf("%s:%d", addr.IP, subset.Ports[portIndex].Port)] = lvs.RealServer{Address: net.ParseIP(addr.IP), Port: uint16(subset.Ports[portIndex].Port), Weight: 1}
			}
		}
	}
	return expectRS
}
