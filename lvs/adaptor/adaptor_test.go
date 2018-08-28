package adaptor

import (
	"net"
	"strconv"
	"testing"

	"github.com/chenchun/kube-bmlb/api"
	"github.com/chenchun/kube-bmlb/lvs"
	lvstesting "github.com/chenchun/kube-bmlb/lvs/testing"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuild(t *testing.T) {
	fake := lvstesting.NewFake()
	vsAddr, noneVsAddr, rsAddr1, rsAddr2 := net.ParseIP("10.0.0.2"), net.ParseIP("10.0.0.3"), net.ParseIP("192.168.0.2"), net.ParseIP("192.168.0.3")
	for _, existVs := range []struct {
		vs  *lvs.VirtualServer
		rss []*lvs.RealServer
	}{
		{
			vs:  &lvs.VirtualServer{Address: vsAddr, Port: 80, Protocol: "TCP"},
			rss: []*lvs.RealServer{{Address: rsAddr1, Port: 81}, {Address: rsAddr2, Port: 82}},
		},
		{
			vs:  &lvs.VirtualServer{Address: noneVsAddr, Port: 80, Protocol: "TCP"},
			rss: []*lvs.RealServer{{Address: rsAddr1, Port: 81}},
		},
	} {
		if err := fake.AddVirtualServer(existVs.vs); err != nil {
			t.Fatal(err)
		}
		for _, rs := range existVs.rss {
			if err := fake.AddRealServer(existVs.vs, rs); err != nil {
				t.Fatal(err)
			}
		}
	}
	str, err := lvs.Dump(fake)
	if err != nil {
		t.Fatal(err)
	}
	if str != `10.0.0.2:80/TCP
  -> 192.168.0.2:81
  -> 192.168.0.3:82

10.0.0.3:80/TCP
  -> 192.168.0.2:81
` {
		t.Fatal(str)
	}
	services := []*v1.Service{
		service("s1", v1.ProtocolTCP, 70, 80),
		service("s2", v1.ProtocolUDP, 8080),
	}
	endpoints := []*v1.Endpoints{
		endpoint("s1", rsAddr1.String(), 71, 81),
		endpoint("s2", rsAddr1.String(), 9000),
		endpoint("s2", rsAddr2.String(), 9001),
	}
	a := &LVSAdaptor{lvsHandler: fake, virtualServerAddress: vsAddr}
	a.Build(services, endpoints, false)
	str, err = lvs.Dump(fake)
	if err != nil {
		t.Fatal(err)
	}
	if str != `10.0.0.2:70/TCP
  -> 192.168.0.2:71

10.0.0.2:80/TCP
  -> 192.168.0.2:81

10.0.0.2:8080/UDP
  -> 192.168.0.2:9000
  -> 192.168.0.3:9001

10.0.0.3:80/TCP
  -> 192.168.0.2:81
` {
		t.Fatal(str)
	}
	// check removeOldVS
	a.Build(services, endpoints, true)
	str, err = lvs.Dump(fake)
	if err != nil {
		t.Fatal(err)
	}
	if str != `10.0.0.2:70/TCP
  -> 192.168.0.2:71

10.0.0.2:80/TCP
  -> 192.168.0.2:81

10.0.0.2:8080/UDP
  -> 192.168.0.2:9000
  -> 192.168.0.3:9001
` {
		t.Fatal(str)
	}
}

func service(name string, proto v1.Protocol, ports ...int) *v1.Service {
	svc := &v1.Service{ObjectMeta: metav1.ObjectMeta{Name: name}}
	portStrs := make([]string, len(ports))
	for i, port := range ports {
		portStrs[i] = strconv.Itoa(port)
		svc.Spec.Ports = append(svc.Spec.Ports, v1.ServicePort{Protocol: proto})
	}
	svc.Annotations = map[string]string{api.ANNOTATION_KEY_PORT: api.EncodeL4Ports(portStrs)}
	return svc
}

func endpoint(name string, ip string, ports ...int32) *v1.Endpoints {
	ep := &v1.Endpoints{ObjectMeta: metav1.ObjectMeta{Name: name}, Subsets: []v1.EndpointSubset{
		{Addresses: []v1.EndpointAddress{{IP: ip}}},
	}}
	for _, p := range ports {
		ep.Subsets[0].Ports = append(ep.Subsets[0].Ports, v1.EndpointPort{Port: p})
	}
	return ep
}
