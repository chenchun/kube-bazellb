package bits

import (
	"sync/atomic"
)

type Bits struct {
	min, max  int32
	allocated []uint32
}

func NewBits(min, max int32) *Bits {
	if max <= min {
		min, max = max, min
	}
	return &Bits{min: min, max: max, allocated: make([]uint32, (max-min)/32+1)}
}

func ordinalToPos(ordinal int32) (int32, int32) {
	return ordinal / 32, ordinal % 32
}

func (a *Bits) Allocate() *int32 {
	for i := int32(a.min); i <= a.max; i++ {
		index, pos := ordinalToPos(i - a.min)
		old := atomic.LoadUint32(&a.allocated[index])
		b := uint32(1) << uint(pos)
		if atomic.CompareAndSwapUint32(&a.allocated[index], old&^b, old|b) {
			return &i
		}
	}
	return nil
}

func (a *Bits) Allocated(i int32) bool {
	if i < a.min || i > a.max {
		return false
	}
	index, pos := ordinalToPos(i - a.min)
	old := atomic.LoadUint32(&a.allocated[index])
	b := uint32(1) << uint(pos)
	return atomic.CompareAndSwapUint32(&a.allocated[index], old&^b, old|b)
}

func (a *Bits) Revoke(i int32) bool {
	if i < a.min || i > a.max {
		return false
	}
	index, pos := ordinalToPos(i - a.min)
	old := atomic.LoadUint32(&a.allocated[index])
	b := uint32(1) << uint(pos)
	return atomic.CompareAndSwapUint32(&a.allocated[index], old|b, old&^b)
}
