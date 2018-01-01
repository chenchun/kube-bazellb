package adaptor

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/chenchun/kube-bmlb/haproxy"
	"k8s.io/api/core/v1"
)

type HAProxyAdaptor struct {
	headerTplt, frontTplt, backTplt *template.Template
}

func NewHAProxyAdaptor() *HAProxyAdaptor {
	return &HAProxyAdaptor{
		headerTplt: template.Must(template.New("header").Parse(haproxy.GetSampleTemplate())),
		frontTplt:  template.Must(template.New("front").Parse(haproxy.GetFrontendTemplate())),
		backTplt:   template.Must(template.New("back").Parse(haproxy.GetBackendTemplate())),
	}
}

func (a *HAProxyAdaptor) Build(lbSvcs []*v1.Service, endpoints []*v1.Endpoints) *bytes.Buffer {
	buf := &bytes.Buffer{}
	a.headerTplt.Execute(buf, nil)
	endpointsMap := map[string]map[string][]*v1.Endpoints{} // Namespace->Name->Endpoints
	for i := range lbSvcs {
		svc := lbSvcs[i]
		if _, ok := endpointsMap[svc.Namespace]; !ok {
			endpointsMap[svc.Namespace] = map[string][]*v1.Endpoints{}
		}
		endpointsMap[svc.Namespace][svc.Name] = []*v1.Endpoints{}
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
	for i := range lbSvcs {
		svc := lbSvcs[i]
		var binds []haproxy.Bind
		for j := range svc.Spec.Ports {
			//TODO concrete the IP once we defined HA
			binds = append(binds, haproxy.Bind{IP: "0.0.0.0", Port: int(svc.Spec.Ports[j].NodePort)})
		}
		a.frontTplt.Execute(buf, haproxy.Frontend{
			Name:           svc.Name,
			Binds:          binds,
			DefaultBackend: svc.Name,
		})
		endpoints := endpointsMap[svc.Namespace][svc.Name]
		if len(endpoints) == 0 {
			continue
		}
		var servers []haproxy.Server
		for j := range endpoints {
			edpt := endpoints[j]
			for k := range edpt.Subsets {
				subset := edpt.Subsets[k]
				for m := range subset.Addresses {
					servers = append(servers, haproxy.Server{
						Name: fmt.Sprintf("%s-%d", svc.Name, len(servers)),
						IP:   subset.Addresses[m].IP,
					})
				}
			}
		}
		a.backTplt.Execute(buf, haproxy.Backend{
			Name:    svc.Name,
			Servers: servers,
		})
	}
	return buf
}
