package adaptor

import "k8s.io/api/core/v1"

type LVSAdaptor struct {
}

func (a *LVSAdaptor) Build(lbSvcs []*v1.Service, endpoints []*v1.Endpoints) {

}
