package flags

import (
	"flag"

	"github.com/spf13/pflag"
)

// ServerRunOptions contains the options while running a server
type ServerRunOptions struct {
	Profiling bool
	Bind      string
	Port      int
	Master    string
	KubeConf  string
	LBType    string
}

var (
	JsonConfigPath string
)

func init() {
	flag.StringVar(&JsonConfigPath, "config", "/etc/sysconfig/kube-bmlb.conf", "The config file location of kube-bmlb")
}

func NewServerRunOptions() *ServerRunOptions {
	return &ServerRunOptions{
		Profiling: true,
		Bind:      "0.0.0.0",
		Port:      9010,
		LBType:    "haproxy",
	}
}

// AddFlags add flags for a specific ASServer to the specified FlagSet
func (s *ServerRunOptions) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&s.Profiling, "profiling", s.Profiling, "Enable profiling via web interface host:port/debug/pprof/")
	fs.StringVar(&s.Bind, "bind", s.Bind, "The ip address to bind")
	fs.IntVar(&s.Port, "port", s.Port, "The port on which to serve")
	fs.StringVar(&s.Master, "master", s.Master, "The address and port of the Kubernetes API server")
	fs.StringVar(&s.KubeConf, "kubeconfig", s.KubeConf, "The kube config file location of APISwitch, used to support TLS")
	fs.StringVar(&s.LBType, "lbtype", s.LBType, "The load balance type, currently supports haproxy and lvs")
}
