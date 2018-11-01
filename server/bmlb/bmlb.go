package bmlb

import (
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"time"

	"github.com/chenchun/kube-bmlb/api"
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
	lb               LoadBalance
	syncChan         chan struct{}
	portAllocator    []port.PortAllocator
}

func NewServer() *Server {
	return &Server{
		ServerRunOptions: flags.NewServerRunOptions(),
		portAllocator:    []port.PortAllocator{port.NewPortAllocator(29000, 29999), port.NewPortAllocator(29000, 29999)}, //tcp, udp
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
		filtered, needsUpdateSvc := s.filterAndAllocatePorts(s.serviceWatcher.List())
		s.lb.Build(filtered, s.endpointsWatcher.List())
		s.updateSvcs(needsUpdateSvc)
	}
}

func protolcolIndex(protocol v1.Protocol) int {
	if protocol == v1.ProtocolUDP {
		return 1
	}
	return 0
}

func protolcol(i int) v1.Protocol {
	if i == 1 {
		return v1.ProtocolUDP
	}
	return v1.ProtocolTCP
}

func (s *Server) filterAndAllocatePorts(svcs []*v1.Service) ([]*v1.Service, map[string]*v1.Service) {
	// keep in mind we may add or del services ports
	var filtered []*v1.Service
	needsUpdateSvc := map[string]*v1.Service{}
	// Mark binded ports in status annotation as allocated
	for i := range svcs {
		svc := svcs[i]
		if svc.Spec.Type != v1.ServiceTypeLoadBalancer {
			continue
		}
		filtered = append(filtered, svc)
		if svc.Annotations == nil {
			continue
		}
		if portStr, exist := svc.Annotations[api.ANStatusBindedPort]; exist {
			protols := api.DecodeL4Ports(portStr)
			for protol, ports := range protols {
				for _, j := range ports {
					s.portAllocator[protol].Allocated(j)
				}
			}
		}
	}
	// Allocate new ports and revoke old ports
	for i := range svcs {
		svc := svcs[i]
		if svc.Spec.Type != v1.ServiceTypeLoadBalancer {
			continue
		}
		needUpdate := false
		allocatedPorts := []map[int32]int32{{}, {}}
		if svc.Annotations != nil {
			if portStr, exist := svc.Annotations[api.ANStatusBindedPort]; exist {
				allocatedPorts = api.DecodeL4Ports(portStr)
			}
		}
		// Allocate new ports
		expectPorts := []map[int32]int32{{}, {}}
		for j := range svc.Spec.Ports {
			port := svc.Spec.Ports[j]
			pi := protolcolIndex(port.Protocol)
			expectPorts[pi][port.Port] = port.Port
			if _, ok := allocatedPorts[pi][port.Port]; ok {
				// port already allocated by this service
				continue
			}
			if !s.portAllocator[pi].Allocated(port.Port) {
				// port has been allocated by another service
				continue
			}
			// successfully allocate this port, we should update service
			needUpdate = true
			allocatedPorts[pi][port.Port] = port.Port
			glog.V(5).Infof("port %v:%d allocated for svc %s", port.Protocol, port.Port, objectKey(&svc.ObjectMeta))
		}
		// revoke old ports
		for protol, ports := range allocatedPorts {
			for _, port := range ports {
				if _, ok := expectPorts[protol][port]; !ok {
					delete(ports, port)
					needUpdate = true
					glog.V(5).Infof("port %v:%d revoked from svc %s", protolcol(protol), port, objectKey(&svc.ObjectMeta))
				}
			}
		}
		if !needUpdate {
			continue
		}
		if len(svc.Status.LoadBalancer.Ingress) > 0 {
			continue
		}
		svc.Status.LoadBalancer.Ingress = []v1.LoadBalancerIngress{{IP: s.Bind}} //TODO load balancer ip
		if svc.Annotations == nil {
			svc.Annotations = map[string]string{}
		}
		svc.Annotations[api.ANStatusBindedPort] = api.EncodeL4Ports(expectPorts)
		needsUpdateSvc[objectKey(&svc.ObjectMeta)] = svc
	}
	return filtered, needsUpdateSvc
}
