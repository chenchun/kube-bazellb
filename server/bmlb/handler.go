package bmlb

import (
	"k8s.io/api/core/v1"
)

func (s *Server) AddService(svc *v1.Service) {
}

func (s *Server) UpdateService(oldSvc, newSvc *v1.Service) {

}

func (s *Server) DeleteService(svc *v1.Service) {
}

func (s *Server) AddEndpoints(ep *v1.Endpoints) {
}

func (s *Server) UpdateEndpoints(oldSvc, newSvc *v1.Endpoints) {

}

func (s *Server) DeleteEndpoints(ep *v1.Endpoints) {
}
