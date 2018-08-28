package adaptor

import (
	"fmt"
	"net"

	"github.com/chenchun/kube-bmlb/api"
	"github.com/chenchun/kube-bmlb/lvs"
	"github.com/docker/libnetwork/ipvs"
	"github.com/golang/glog"
	"k8s.io/api/core/v1"
)

type LVSAdaptor struct {
	lvsHandler           lvs.Interface
	virtualServerAddress net.IP
}

func NewLVSAdaptor(virtualServerAddress net.IP) *LVSAdaptor {
	return &LVSAdaptor{lvsHandler: lvs.New(), virtualServerAddress: virtualServerAddress}
}

func (a *LVSAdaptor) Build(lbSvcs []*v1.Service, endpoints []*v1.Endpoints, removeOldVS bool) {
	endpointsMap := map[string]map[string][]*v1.Endpoints{} // Namespace->Name->Endpoints
	// virtual server is like 10.0.0.2:8080, service has allocated ports in annotation
	// so build a map which maps ports to service
	portServiceMap := []map[int]*v1.Service{{}, {}} //protocol port:service
	for i := range lbSvcs {
		svc := lbSvcs[i]
		if _, ok := endpointsMap[svc.Namespace]; !ok {
			endpointsMap[svc.Namespace] = map[string][]*v1.Endpoints{}
		}
		endpointsMap[svc.Namespace][svc.Name] = []*v1.Endpoints{}
		lbPorts := api.DecodeL4Ports(svc.Annotations[api.ANNOTATION_KEY_PORT])
		if len(lbPorts) != len(svc.Spec.Ports) {
			glog.Errorf("loadbalance ports size %d not equal service ports size %d", len(lbPorts), len(svc.Spec.Ports))
			return
		}
		for j, port := range svc.Spec.Ports {
			var index = 0
			if port.Protocol == v1.ProtocolUDP {
				index = 1
			}
			portServiceMap[index][lbPorts[j]] = svc
		}
	}
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
	for _, vs := range vss {
		var index = 0
		if vs.Protocol == "UDP" {
			index = 1
		}
		if svc, ok := portServiceMap[index][int(vs.Port)]; !ok {
			// service not exists, but virtual server exists
			if removeOldVS {
				if err := a.lvsHandler.DeleteVirtualServer(vs); err != nil {
					// raise a warning instead of error as we will retry later
					glog.Warningf("failed to delete virtual server %s: %v", vs.String(), err)
				}
			}
		} else {
			delete(portServiceMap[index], int(vs.Port))
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
			for _, rs := range rss {
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
	lbPorts := api.DecodeL4Ports(svc.Annotations[api.ANNOTATION_KEY_PORT])
	portIndex := -1
	for i, p := range lbPorts {
		if uint16(p) == vs.Port {
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
				if len(subset.Ports) != len(lbPorts) {
					// endpoint is not synced with service yet
					continue
				}
				expectRS[fmt.Sprintf("%s:%d", addr.IP, subset.Ports[portIndex].Port)] = lvs.RealServer{Address: net.ParseIP(addr.IP), Port: uint16(subset.Ports[portIndex].Port)}
			}
		}
	}
	return expectRS
}
