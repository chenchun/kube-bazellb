package port

import (
	"sync/atomic"

	"github.com/golang/glog"
)

// PortAllocator allocates ports and is thread safe
type PortAllocator struct {
	min       uint
	allocated []uint32
}

func NewPortAllocator(min, max uint) *PortAllocator {
	// kubernetes nodeport binds to (default: 30000-32767)
	if max <= min || max >= 30000 {
		glog.Fatal("max should bigger than min and smaller than 30000")
	}
	return &PortAllocator{min: min, allocated: make([]uint32, max-min+1)}
}

func (a *PortAllocator) Allocate() uint {
	for i := range a.allocated {
		if atomic.CompareAndSwapUint32(&a.allocated[i], 0, 1) {
			return uint(i) + a.min
		}
	}
	return 0
}

func (a *PortAllocator) Allocated(port uint) {
	if port < a.min || port-a.min > uint(len(a.allocated)-1) {
		return
	}
	atomic.CompareAndSwapUint32(&a.allocated[port-a.min], 0, 1)
}

func (a *PortAllocator) Revoke(port uint) {
	if port < a.min || port-a.min > uint(len(a.allocated)-1) {
		return
	}
	atomic.CompareAndSwapUint32(&a.allocated[port-a.min], 1, 0)
}
