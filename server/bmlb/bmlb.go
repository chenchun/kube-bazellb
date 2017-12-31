package bmlb

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"time"

	"github.com/chenchun/kube-bmlb/haproxy"
	"github.com/chenchun/kube-bmlb/server/flags"
	"github.com/chenchun/kube-bmlb/watch"
	"github.com/golang/glog"
	"github.com/spf13/pflag"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type Server struct {
	*flags.ServerRunOptions
	serviceWatcher   *watch.ServiceWatcher
	endpointsWatcher *watch.EndpointsWatcher
	Client           *kubernetes.Clientset
	haproxy          *haproxy.Haproxy
}

func NewServer() *Server {
	return &Server{
		ServerRunOptions: flags.NewServerRunOptions(),
		haproxy:          haproxy.NewHaproxy(),
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
	if err := s.haproxy.Start(); err != nil {
		glog.Fatalf("failed to start haproxy: %v", err)
	}
	if err := s.startServer(); err != nil {
		glog.Fatalf("failed to start server: %v", err)
	}
}

func (s *Server) startWatcher() {
	if s.Master == "" && s.KubeConf == "" {
		glog.Warning("Master/KubeConf both empty, won't connecting to kube-apiserver")
		return
	}
	clientConfig, err := clientcmd.BuildConfigFromFlags(s.Master, s.KubeConf)
	if err != nil {
		glog.Fatalf("Invalid client config: %v", err)
	}
	clientConfig.QPS = 1000.0
	clientConfig.Burst = 2000
	glog.Infof("QPS: %e, Burst: %d", clientConfig.QPS, clientConfig.Burst)
	s.Client, err = kubernetes.NewForConfig(clientConfig)
	if err != nil {
		glog.Fatalf("Can not generate client from config: %v", err)
	}
	s.serviceWatcher = watch.StartServiceWatcher(s.Client, 5*time.Minute, s)
	s.endpointsWatcher = watch.StartEndpointsWatcher(s.Client, 5*time.Minute, s)
}

func (s *Server) startServer() error {
	glog.Infof("starting http server")
	return http.ListenAndServe(fmt.Sprintf(":%d", s.Port), nil)
}
