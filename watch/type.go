package watch

import "k8s.io/api/core/v1"

type ServiceHandler interface {
	AddService(svc *v1.Service)
	DeleteService(svc *v1.Service)
	UpdateService(oldSvc, newSvc *v1.Service)
}

type EndpointsHandler interface {
	AddEndpoints(ep *v1.Endpoints)
	DeleteEndpoints(ep *v1.Endpoints)
	UpdateEndpoints(oldEp, newEp *v1.Endpoints)
}
