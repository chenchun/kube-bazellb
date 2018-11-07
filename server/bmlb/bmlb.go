package bmlb

import (
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"time"

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
	lb               LoadBalance
	syncChan         chan struct{}
}

func NewServer() *Server {
	return &Server{
		ServerRunOptions: flags.NewServerRunOptions(),
	}
}

// AddFlags adds flags for a specific Server to the specified FlagSet
func (s *Server) AddFlags(fs *pflag.FlagSet) {
	// Add the generic flags.
	s.ServerRunOptions.AddFlags(fs)
}

func (s *Server) Init() {
	ip := net.ParseIP(s.Bind)
	if ip == nil {
		glog.Fatal("bind address is invalid: %s", s.Bind)
	}
	s.lb = NewLoadBalance(s.LBType, ip)
}

func (s *Server) Start() {
	s.Init()
	s.startWatcher()
	go s.lb.Run(struct{}{})
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
	s.serviceWatcher = watch.StartServiceWatcher(s.Client, 0, s)
	s.endpointsWatcher = watch.StartEndpointsWatcher(s.Client, 0, s)
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
	s.syncChan = make(chan struct{}, 2)
	s.syncChan <- struct{}{}
	tick := time.Tick(time.Minute)
	for {
		select {
		case <-s.syncChan:
		case <-tick:
		}
		//TODO incremental
		filtered, needsUpdate := s.filter(s.serviceWatcher.List())
		s.lb.Build(filtered, s.endpointsWatcher.List())
		s.updateSvcs(needsUpdate)
	}
}

func (s *Server) filter(svcs []*v1.Service) ([]*v1.Service, []*v1.Service) {
	// keep in mind we may add or del services ports
	var filtered, needsUpdate []*v1.Service
	for i := range svcs {
		svc := svcs[i]
		if svc.Spec.Type != v1.ServiceTypeLoadBalancer {
			continue
		}
		filtered = append(filtered, svc)
		findLBIP := false
		for _, ingress := range svc.Status.LoadBalancer.Ingress {
			if ingress.IP == s.Bind {
				findLBIP = true
				break
			}
		}
		if !findLBIP {
			needsUpdate = append(needsUpdate, svc)
			svc.Status.LoadBalancer.Ingress = append(svc.Status.LoadBalancer.Ingress, v1.LoadBalancerIngress{IP: s.Bind})
		}
	}
	return filtered, needsUpdate
}
