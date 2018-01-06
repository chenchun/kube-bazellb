package bmlb

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/chenchun/kube-bmlb/api"
	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
)

func (s *Server) AddService(svc *v1.Service) {
	glog.V(5).Infof("add svc %s", objectKey(&svc.ObjectMeta))
	s.maybeSync()
}

func (s *Server) UpdateService(oldSvc, newSvc *v1.Service) {
	if s.skipServiceUpdate(oldSvc, newSvc) {
		return
	}
	glog.V(5).Infof("update svc %s", objectKey(&newSvc.ObjectMeta))
	s.maybeSync()
}

func (s *Server) DeleteService(svc *v1.Service) {
	ports := api.DecodeL4Ports(svc.Annotations[api.ANNOTATION_KEY_PORT])
	for i := range ports {
		s.portAllocator.Revoke(uint(ports[i]))
	}
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

func (s *Server) skipServiceUpdate(old, new *v1.Service) bool {
	f := func(svc *v1.Service) *v1.Service {
		p := svc.DeepCopy()
		// ResourceVersion must be excluded because each object update will
		// have a new resource version.
		p.ResourceVersion = ""
		// ANNOTATION_KEY_PORT must be excluded
		p.Annotations[api.ANNOTATION_KEY_PORT] = ""
		return p
	}
	oldCopy, newCopy := f(old), f(new)
	if !reflect.DeepEqual(oldCopy, newCopy) {
		return false
	}
	glog.V(3).Infof("Skipping service %s update", objectKey(&new.ObjectMeta))
	return true
}

func objectKey(om *metav1.ObjectMeta) string {
	return fmt.Sprintf("%s_%s", om.Name, om.Namespace)
}

func (s *Server) updateSvcs(svcs map[string]*v1.Service) {
	if len(svcs) > 0 {
		glog.V(3).Infof("updating svc %v", svcs)
	}
	var wg sync.WaitGroup
	for i := range svcs {
		wg.Add(1)
		go func(svc *v1.Service) {
			defer wg.Done()
			svcCopy := svc.DeepCopy()
			if svcCopy.Annotations == nil {
				svcCopy.Annotations = map[string]string{}
			}
			svcCopy.Annotations[api.ANNOTATION_KEY_PORT] = svc.Annotations[api.ANNOTATION_KEY_PORT]
			ret := &unstructured.Unstructured{}
			ret.SetAnnotations(svcCopy.Annotations)
			patchData, err := json.Marshal(ret)
			if err != nil {
				glog.Error(err)
			}
			if err := wait.PollImmediate(time.Second, 2*time.Minute, func() (bool, error) {
				_, err := s.Client.CoreV1().Services(svcCopy.Namespace).Patch(svcCopy.Name, types.MergePatchType, patchData)
				if err != nil {
					glog.Warningf("failed to update svc %s: %v", objectKey(&svc.ObjectMeta), err)
					return false, nil
				}
				glog.V(3).Infof("updated %s for svc %s", svcCopy.Annotations[api.ANNOTATION_KEY_PORT], objectKey(&svc.ObjectMeta))
				return true, nil
			}); err != nil {
				glog.Errorf("failed to update svc %s: %v", objectKey(&svc.ObjectMeta), err)
			}
		}(svcs[i])
	}
	wg.Wait()
}
