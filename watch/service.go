package watch

import (
	"time"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type ServiceWatcher struct {
	serviceController cache.Controller
	serviceLister     cache.Indexer
	eventHandler      ServiceHandler
}

func (w *ServiceWatcher) serviceAddEventHandler(obj interface{}) {
	service, ok := obj.(*v1.Service)
	if !ok {
		return
	}
	w.eventHandler.AddService(service)
}

func (w *ServiceWatcher) serviceDeleteEventHandler(obj interface{}) {
	service, ok := obj.(*v1.Service)
	if !ok {
		return
	}
	w.eventHandler.DeleteService(service)
}

func (w *ServiceWatcher) serviceUpdateEventHandler(oldObj, newObj interface{}) {
	service, ok := newObj.(*v1.Service)
	if !ok {
		return
	}
	oldService, ok := oldObj.(*v1.Service)
	if !ok {
		return
	}
	w.eventHandler.UpdateService(oldService, service)
}

func (svcw *ServiceWatcher) List() []*v1.Service {
	obj_list := svcw.serviceLister.List()
	svc_instances := make([]*v1.Service, len(obj_list))
	for i, ins := range obj_list {
		svc_instances[i] = ins.(*v1.Service)
	}
	return svc_instances
}

func (svcw *ServiceWatcher) HasSynced() bool {
	return svcw.serviceController.HasSynced()
}

var servicesStopCh chan struct{}

func StartServiceWatcher(client *kubernetes.Clientset, resyncPeriod time.Duration, sh ServiceHandler) *ServiceWatcher {
	w := ServiceWatcher{eventHandler: sh}
	lw := cache.NewListWatchFromClient(client.Core().RESTClient(), "services", metav1.NamespaceAll, fields.OneTermEqualSelector("spec.type", "LoadBalancer"))
	w.serviceLister, w.serviceController = cache.NewIndexerInformer(
		lw,
		&v1.Service{},
		resyncPeriod,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    w.serviceAddEventHandler,
			DeleteFunc: w.serviceDeleteEventHandler,
			UpdateFunc: w.serviceUpdateEventHandler,
		},
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)
	servicesStopCh = make(chan struct{})
	go w.serviceController.Run(servicesStopCh)
	return &w
}

func StopServiceWatcher() {
	servicesStopCh <- struct{}{}
}
