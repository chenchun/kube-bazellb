package bmlb

import (
	"net"

	"k8s.io/api/core/v1"
	"github.com/chenchun/kube-bmlb/haproxy"
	haproxyAdaptor "github.com/chenchun/kube-bmlb/haproxy/adaptor"
	lvsAdaptor "github.com/chenchun/kube-bmlb/lvs/adaptor"
	"github.com/golang/glog"
)

func NewLoadBalance(lbtype string, ip net.IP) LoadBalance {
	switch lbtype {
	case "haproxy":
		return &HaproxyLB{
			haproxy: haproxy.NewHaproxy(),
			adaptor: haproxyAdaptor.NewHAProxyAdaptor()}
	case "lvs":
		return &LVSLB{adaptor: lvsAdaptor.NewLVSAdaptor(ip)}
	default:
		glog.Fatal("unsupport lbtype: %s", lbtype)
	}
	return nil
}

type LoadBalance interface {
	SupportIncrementalUpdate() bool
	Build(lbSvcs []*v1.Service, endpoints []*v1.Endpoints, incremental bool)
	Run(stop struct{})
}

type HaproxyLB struct {
	haproxy *haproxy.Haproxy
	adaptor *haproxyAdaptor.HAProxyAdaptor
}

func (h *HaproxyLB) SupportIncrementalUpdate() bool {
	return false
}

func (h *HaproxyLB) Build(lbSvcs []*v1.Service, endpoints []*v1.Endpoints, incremental bool) {
	if incremental {
		return
	}
	buf := h.adaptor.Build(lbSvcs, endpoints)
	h.haproxy.ConfigChan <- buf
}

func (h *HaproxyLB) Run(stop struct{}) {
	h.haproxy.Run()
}

type LVSLB struct {
	adaptor *lvsAdaptor.LVSAdaptor
}

func (h *LVSLB) SupportIncrementalUpdate() bool {
	return true
}

func (h *LVSLB) Build(lbSvcs []*v1.Service, endpoints []*v1.Endpoints, incremental bool) {
	h.adaptor.Build(lbSvcs, endpoints, incremental)
}

func (h *LVSLB) Run(stop struct{}) {

}


