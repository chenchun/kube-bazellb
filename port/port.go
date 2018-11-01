package port

import (
	"github.com/chenchun/kube-bmlb/utils/bits"
	"github.com/golang/glog"
)

type PortAllocator interface {
	Allocate() *int32
	Allocated(port int32) bool
	Revoke(port int32) bool
}

// portAllocator allocates ports and is thread safe
type portAllocator struct {
	bs *bits.Bits
}

func NewPortAllocator(min, max int32) PortAllocator {
	// kubernetes nodeport binds to (default: 30000-32767)
	if max <= min || max >= 30000 {
		glog.Fatal("max should bigger than min and smaller than 30000")
	}
	return &portAllocator{bs: bits.NewBits(min, max)}
}

func (a *portAllocator) Allocate() *int32 {
	return a.bs.Allocate()
}

func (a *portAllocator) Allocated(port int32) bool {
	return a.bs.Allocated(port)
}

func (a *portAllocator) Revoke(port int32) bool {
	return a.bs.Revoke(port)
}
