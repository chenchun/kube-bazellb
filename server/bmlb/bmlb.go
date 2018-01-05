package bmlb

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"strconv"
	"time"

	"github.com/chenchun/kube-bmlb/api"
	"github.com/chenchun/kube-bmlb/haproxy"
	"github.com/chenchun/kube-bmlb/haproxy/adaptor"
	"github.com/chenchun/kube-bmlb/port"
	"github.com/chenchun/kube-bmlb/server/flags"
	"github.com/chenchun/kube-bmlb/watch"
	"github.com/golang/glog"
	"github.com/spf13/pflag"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type Server struct {
	*flags.ServerRunOptions
	serviceWatcher   *watch.ServiceWatcher
	endpointsWatcher *watch.EndpointsWatcher
	Client           *kubernetes.Clientset
	haproxy          *haproxy.Haproxy
	adaptor          *adaptor.HAProxyAdaptor
	syncChan         chan struct{}
	portAllocator    *port.PortAllocator
}

func NewServer() *Server {
	return &Server{
		ServerRunOptions: flags.NewServerRunOptions(),
		haproxy:          haproxy.NewHaproxy(),
		adaptor:          adaptor.NewHAProxyAdaptor(),
		portAllocator:    port.NewPortAllocator(29000, 29999),
	}
}

// AddFlags adds flags for a specific Server to the specified FlagSet
func (s *Server) AddFlags(fs *pflag.FlagSet) {
	// Add the generic flags.
	s.ServerRunOptions.AddFlags(fs)
}

func (s *Server) Init() {}

func (s *Server) Start() {
	s.Init()
	s.startWatcher()
	go s.haproxy.Run()
	go s.syncing()
	if err := s.launchServer(); err != nil {
		glog.Fatalf("failed to start server: %v", err)
	}
}

func (s *Server) startWatcher() {
	glog.Infof("connecting to kube-apiserver with master %q, kubeconf %q", s.Master, s.KubeConf)
	clientConfig, err := clientcmd.BuildConfigFromFlags(s.Master, s.KubeConf)
	if err != nil {
		glog.Fatalf("Invalid client config: %v", err)
	}
	clientConfig.QPS = 1e6
	clientConfig.Burst = 1e6

	s.Client, err = kubernetes.NewForConfig(clientConfig)
	if err != nil {
		glog.Fatalf("Can not generate client from config: %v", err)
	}
	v, err := s.Client.Discovery().ServerVersion()
	if err != nil {
		glog.Fatal(err)
	}
	glog.Infof("Running in Kubernetes Cluster version v%v.%v (%v) - git (%v) commit %v - platform %v",
		v.Major, v.Minor, v.GitVersion, v.GitTreeState, v.GitCommit, v.Platform)
	s.serviceWatcher = watch.StartServiceWatcher(s.Client, 5*time.Minute, s)
	s.endpointsWatcher = watch.StartEndpointsWatcher(s.Client, 5*time.Minute, s)
}

func (s *Server) launchServer() error {
	glog.Infof("starting http server")
	return http.ListenAndServe(fmt.Sprintf(":%d", s.Port), nil)
}

func (s *Server) syncing() {
	wait.PollInfinite(time.Second, func() (done bool, err error) {
		glog.V(3).Infof("waiting for syncing service/endpoints")
		return s.serviceWatcher.HasSynced() && s.endpointsWatcher.HasSynced(), nil
	})
	s.syncChan = make(chan struct{})
	tick := time.Tick(10 * time.Minute)
	for {
		select {
		case <-s.syncChan:
		case <-tick:
		}
		filtered, needsUpdateSvc := s.filterAndAllocatePorts(s.serviceWatcher.List())
		buf := s.adaptor.Build(filtered, s.endpointsWatcher.List())
		s.haproxy.ConfigChan <- buf
		s.updateSvcs(needsUpdateSvc)
	}
}

func (s *Server) filterAndAllocatePorts(svcs []*v1.Service) ([]*v1.Service, map[string]*v1.Service) {
	var filtered []*v1.Service
	needsUpdateSvc := map[string]*v1.Service{}
	for i := range svcs {
		svc := svcs[i]
		if svc.Spec.Type != v1.ServiceTypeLoadBalancer {
			continue
		}
		if svc.Annotations == nil {
			svc.Annotations = map[string]string{}
		}
		if portStr, exist := svc.Annotations[api.ANNOTATION_KEY_PORT]; !exist {
			allocated := make([]string, len(svc.Spec.Ports))
			for j := range svc.Spec.Ports {
				allocated[j] = strconv.Itoa(int(s.portAllocator.Allocate()))
			}
			svc.Annotations[api.ANNOTATION_KEY_PORT] = api.EncodeL4Ports(allocated)
			needsUpdateSvc[serviceKey(svc)] = svc
		} else {
			ports := api.DecodeL4Ports(portStr)
			for j := range ports {
				s.portAllocator.Allocated(uint(ports[j]))
			}
		}
		filtered = append(filtered, svc)
	}
	return filtered, needsUpdateSvc
}
