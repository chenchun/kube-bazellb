package adaptor

import (
	"fmt"
	"net"

	"github.com/chenchun/kube-bmlb/lvs"
	"github.com/chenchun/kube-bmlb/utils/dbus"
	"github.com/chenchun/kube-bmlb/utils/ipset"
	"github.com/chenchun/kube-bmlb/utils/iptables"
	"github.com/chenchun/kube-bmlb/utils/sysctl"
	"github.com/docker/libnetwork/ipvs"
	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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
	portServiceMap := []map[int32][]*v1.Service{{}, {}} //protocol port:service array
	for i := range lbSvcs {
		svc := lbSvcs[i]
		if _, ok := endpointsMap[svc.Namespace]; !ok {
			endpointsMap[svc.Namespace] = map[string][]*v1.Endpoints{}
		}
		endpointsMap[svc.Namespace][svc.Name] = []*v1.Endpoints{}
		for _, port := range svc.Spec.Ports {
			protocolMap := portServiceMap[protolcolIndex(port.Protocol)]
			protocolMap[port.Port] = append(protocolMap[port.Port], svc)
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
		if svcs, ok := portServiceMap[index][int32(vs.Port)]; !ok {
			// service not exists, but virtual server exists
			if err := a.lvsHandler.DeleteVirtualServer(vs); err != nil {
				// raise a warning instead of error as we will retry later
				glog.Warningf("failed to delete virtual server %s: %v", vs.String(), err)
			}
		} else {
			delete(portServiceMap[index], int32(vs.Port))
			// syncing real servers
			expectRSs := getExpectRSs(svcs, endpointsMap, vs)
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
			a.addRealServers(vs, expectRSs)
		}
	}

	// create not exist virtual services and real servers
	for i := 0; i < len(portServiceMap); i++ {
		protocol := "TCP"
		if i == 1 {
			protocol = "UDP"
		}
		for port, svcs := range portServiceMap[i] {
			vs := &lvs.VirtualServer{Address: a.virtualServerAddress, Port: uint16(port), Protocol: protocol, Scheduler: ipvs.RoundRobin}
			if err := a.lvsHandler.AddVirtualServer(vs); err != nil {
				// raise a warning instead of error as we will retry later
				glog.Warningf("failed to add virtual server %s: %v", vs.String(), err)
				continue
			}
			a.addRealServers(vs, getExpectRSs(svcs, endpointsMap, vs))
		}
	}
}

func (a *LVSAdaptor) addRealServers(vs *lvs.VirtualServer, expectRSs map[string]lvs.RealServer) {
	for str := range expectRSs {
		expectRS := expectRSs[str]
		if err := a.lvsHandler.AddRealServer(vs, &expectRS); err != nil {
			glog.Warningf("failed to add real server %s: %v", expectRS.String(), err)
		}
	}
}

func getExpectRSs(svcs []*v1.Service, endpointsMap map[string]map[string][]*v1.Endpoints, vs *lvs.VirtualServer) map[string]lvs.RealServer {
	expectRSs := map[string]lvs.RealServer{}
	// syncing real servers
	for _, svc := range svcs {
		edpts := endpointsMap[svc.Namespace][svc.Name]
		if len(edpts) == 0 {
			continue
		}
		addExpectRS(expectRSs, edpts, vs, svc)
	}
	return expectRSs
}

func addExpectRS(expectRS map[string]lvs.RealServer, edpts []*v1.Endpoints, vs *lvs.VirtualServer, svc *v1.Service) {
	var targetPort *intstr.IntOrString
	for _, p := range svc.Spec.Ports {
		if uint16(p.Port) == vs.Port {
			targetPort = &p.TargetPort
			break
		}
	}
	if targetPort == nil {
		// should never happen
		return
	}
	for _, edpt := range edpts {
		for _, subset := range edpt.Subsets {
			port := getTargetIntPort(targetPort, &subset)
			if port == 0 {
				// endpoints may not have been synced
				continue
			}
			for _, addr := range subset.Addresses {
				expectRS[fmt.Sprintf("%s:%d", addr.IP, port)] = lvs.RealServer{Address: net.ParseIP(addr.IP), Port: uint16(port), Weight: 1}
			}
		}
	}
	return
}

func getTargetPort(port int32, svc *v1.Service) *intstr.IntOrString {
	var targetPort *intstr.IntOrString
	for _, p := range svc.Spec.Ports {
		if p.Port == port {
			targetPort = &p.TargetPort
			break
		}
	}
	return targetPort
}

func getTargetIntPort(targetPort *intstr.IntOrString, subset *v1.EndpointSubset) int32 {
	if targetPort.Type == intstr.Int {
		return targetPort.IntVal
	}
	for _, port := range subset.Ports {
		if port.Name == targetPort.StrVal {
			return port.Port
		}
	}
	return 0
}

func protolcolIndex(protocol v1.Protocol) int {
	if protocol == v1.ProtocolUDP {
		return 1
	}
	return 0
}

func protolcol(i int) v1.Protocol {
	if i == 1 {
		return v1.ProtocolUDP
	}
	return v1.ProtocolTCP
}
