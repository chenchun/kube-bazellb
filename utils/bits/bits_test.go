package bits

import (
	"math/rand"
	"testing"
)

func TestBits1(t *testing.T) {
	b := NewBits(3, 40)
	for _, n := range []int32{3, 4, 34, 35, 40} {
		if !b.Allocated(n) {
			t.Fatal()
		}
	}
	if b.Allocated(2) || b.Allocated(41) {
		t.Fatal()
	}
	for _, n := range []int32{3, 4, 34, 35, 40} {
		if !b.Revoke(n) {
			t.Fatal()
		}
	}
}

func TestBits2(t *testing.T) {
	b := NewBits(3, 400)
	perm := rand.Perm(397)
	for _, n := range perm {
		if !b.Allocated(int32(n) + 3) {
			t.Fatal()
		}
	}
	perm = rand.Perm(397)
	for _, n := range perm {
		if !b.Revoke(int32(n) + 3) {
			t.Fatal()
		}
	}
}

func TestBits3(t *testing.T) {
	b := NewBits(3, 9)
	for _, n := range []int32{3, 4, 7, 8} {
		if !b.Allocated(n) {
			t.Fatal()
		}
	}
	for _, n := range []int32{5, 6, 9} {
		alloc := b.Allocate()
		if alloc == nil || *alloc != n {
			t.Fatalf("real %d, expect %d", *alloc, n)
		}
	}
	if b.Allocate() != nil {
		t.Fatal()
	}
	b = NewBits(3, 400)
	for i := int32(3); i <= 400; i++ {
		alloc := b.Allocate()
		if alloc == nil || *alloc != i {
			t.Fatal(alloc)
		}
	}
}
