package adaptor

import (
	"bytes"
	"net"
	"testing"

	"github.com/chenchun/kube-bmlb/api"
	"github.com/chenchun/kube-bmlb/lvs"
	lvstesting "github.com/chenchun/kube-bmlb/lvs/testing"
	ipsettesting "github.com/chenchun/kube-bmlb/utils/ipset/testing"
	"github.com/chenchun/kube-bmlb/utils/iptables"
	ipttesting "github.com/chenchun/kube-bmlb/utils/iptables/testing"
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
	a := &LVSAdaptor{lvsHandler: fake, virtualServerAddress: vsAddr, iptHandler: ipttesting.NewFakeIPTables(), ipsetHandler: ipsettesting.NewFake("")}
	a.Build(services, endpoints)
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

	// check iptables and ipset
	buf := bytes.NewBuffer(nil)
	if err := a.iptHandler.SaveInto(iptables.TableNAT, buf); err != nil {
		t.Fatal(err)
	} else {
		if buf.String() != `*nat
:INPUT - [0:0]
:OUTPUT - [0:0]
:POSTROUTING - [0:0]
:PREROUTING - [0:0]
-A OUTPUT -p all -m set --match-set bmlb-vip-vport dst,dst -j MARK --set-xmark 0x4000/0x4000
-A POSTROUTING -m mark --mark 0x4000/0x4000 -j MASQUERADE
-A PREROUTING -p all -m set --match-set bmlb-vip-vport dst,dst -j MARK --set-xmark 0x4000/0x4000
COMMIT
` {
			t.Fatal(buf.String())
		}
	}
	if data, err := a.ipsetHandler.SaveAllSets(); err != nil {
		t.Fatal(err)
	} else {
		if string(data) != `Name: bmlb-vip-vport
Type: hash:ip,port
Members:
10.0.0.2,tcp:70
10.0.0.2,tcp:80
10.0.0.2,udp:8080
` {
			t.Fatal(string(data))
		}
	}
}

func service(name string, proto v1.Protocol, ports ...int) *v1.Service {
	svc := &v1.Service{ObjectMeta: metav1.ObjectMeta{Name: name}}
	portMap := map[int32]int32{}
	for _, port := range ports {
		portMap[int32(port)] = int32(port)
		svc.Spec.Ports = append(svc.Spec.Ports, v1.ServicePort{Protocol: proto})
	}
	protocolPorts := []map[int32]int32{portMap, {}}
	if proto == v1.ProtocolUDP {
		protocolPorts = []map[int32]int32{{}, portMap}
	}
	svc.Annotations = map[string]string{api.ANStatusBindedPort: api.EncodeL4Ports(protocolPorts)}
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
