package watch

import (
	"time"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type EndpointsWatcher struct {
	endpointsController cache.Controller
	endpointsLister     cache.Indexer
	endpointsHandler    EndpointsHandler
}

func (w *EndpointsWatcher) endpointsAddEventHandler(obj interface{}) {
	endpoints, ok := obj.(*v1.Endpoints)
	if !ok {
		return
	}
	w.endpointsHandler.AddEndpoints(endpoints)
}

func (w *EndpointsWatcher) endpointsDeleteEventHandler(obj interface{}) {
	endpoints, ok := obj.(*v1.Endpoints)
	if !ok {
		return
	}
	w.endpointsHandler.DeleteEndpoints(endpoints)
}

func (w *EndpointsWatcher) endpointsUpdateEventHandler(oldObj, newObj interface{}) {
	endpoints, ok := newObj.(*v1.Endpoints)
	if !ok {
		return
	}
	oldEndpoints, ok := oldObj.(*v1.Endpoints)
	if !ok {
		return
	}
	w.endpointsHandler.UpdateEndpoints(oldEndpoints, endpoints)
}

func (ew *EndpointsWatcher) List() []*v1.Endpoints {
	obj_list := ew.endpointsLister.List()
	ep_instances := make([]*v1.Endpoints, len(obj_list))
	for i, ins := range obj_list {
		ep_instances[i] = ins.(*v1.Endpoints)
	}
	return ep_instances
}

func (ew *EndpointsWatcher) HasSynced() bool {
	return ew.endpointsController.HasSynced()
}

var endpointsStopCh chan struct{}

func StartEndpointsWatcher(clientset *kubernetes.Clientset, resyncPeriod time.Duration, h EndpointsHandler) *EndpointsWatcher {
	ew := EndpointsWatcher{endpointsHandler: h}
	lw := cache.NewListWatchFromClient(clientset.Core().RESTClient(), "endpoints", metav1.NamespaceAll, fields.Everything())
	ew.endpointsLister, ew.endpointsController = cache.NewIndexerInformer(
		lw,
		&v1.Endpoints{},
		resyncPeriod,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    ew.endpointsAddEventHandler,
			DeleteFunc: ew.endpointsDeleteEventHandler,
			UpdateFunc: ew.endpointsUpdateEventHandler,
		},
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)
	endpointsStopCh = make(chan struct{})
	go ew.endpointsController.Run(endpointsStopCh)
	return &ew
}

func StopEndpointsWatcher() {
	endpointsStopCh <- struct{}{}
}
