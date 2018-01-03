package bmlb

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"time"

	"github.com/chenchun/kube-bmlb/haproxy"
	"github.com/chenchun/kube-bmlb/haproxy/adaptor"
	"github.com/chenchun/kube-bmlb/server/flags"
	"github.com/chenchun/kube-bmlb/watch"
	"github.com/golang/glog"
	"github.com/spf13/pflag"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/kubernetes/pkg/util/wait"
)

type Server struct {
	*flags.ServerRunOptions
	serviceWatcher   *watch.ServiceWatcher
	endpointsWatcher *watch.EndpointsWatcher
	Client           *kubernetes.Clientset
	haproxy          *haproxy.Haproxy
	adaptor          *adaptor.HAProxyAdaptor
	hasSynced        bool
}

func NewServer() *Server {
	return &Server{
		ServerRunOptions: flags.NewServerRunOptions(),
		haproxy:          haproxy.NewHaproxy(),
		adaptor:          adaptor.NewHAProxyAdaptor(),
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
	wait.Forever(s.syncing, time.Hour)
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
	if !s.hasSynced && (!s.serviceWatcher.HasSynced() || !s.endpointsWatcher.HasSynced()) {
		glog.V(3).Infof("waiting for syncing service/endpoints")
		return
	}
	buf := s.adaptor.Build(s.serviceWatcher.List(), s.endpointsWatcher.List())
	s.haproxy.ConfigChan <- buf
	s.hasSynced = true
}
