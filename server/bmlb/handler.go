package bmlb

import (
	"fmt"

	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (s *Server) AddService(svc *v1.Service) {
	glog.V(5).Infof("add svc %s", objectKey(&svc.ObjectMeta))
	s.maybeSync()
}

func (s *Server) UpdateService(oldSvc, newSvc *v1.Service) {
	glog.V(5).Infof("update svc %s", objectKey(&newSvc.ObjectMeta))
	s.maybeSync()
}

func (s *Server) DeleteService(svc *v1.Service) {
	glog.V(5).Infof("delete svc %s", objectKey(&svc.ObjectMeta))
	s.maybeSync()
}

func (s *Server) AddEndpoints(ep *v1.Endpoints) {
	glog.V(5).Infof("add endpoints %s", objectKey(&ep.ObjectMeta))
	s.maybeSync()
}

func (s *Server) UpdateEndpoints(oldEp, newEp *v1.Endpoints) {
	glog.V(5).Infof("update endpoints %s", objectKey(&newEp.ObjectMeta))
	s.maybeSync()
}

func (s *Server) DeleteEndpoints(ep *v1.Endpoints) {
	glog.V(5).Infof("delete endpoints %s", objectKey(&ep.ObjectMeta))
	s.maybeSync()
}

func (s *Server) maybeSync() {
	if s.syncChan != nil {
		select {
		case s.syncChan <- struct{}{}:
		default:
			glog.V(4).Infof("sync chan has waiting job, no need to create another one")
		}
	}
}

func objectKey(om *metav1.ObjectMeta) string {
	return fmt.Sprintf("%s_%s", om.Name, om.Namespace)
}
