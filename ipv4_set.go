package main

import (
	"math/bits"
)

const (
	TotalIPs    = 1 << 32       // total number of unique IPs
	Words       = TotalIPs >> 6 // set size
	BitmapBytes = Words * 8     // 536,870,912 bytes = 512MB
)

type ipv4Set struct {
	bitmap []uint64
}

func newIPv4Set() *ipv4Set {
	bm := make([]uint64, Words)
	return &ipv4Set{bitmap: bm}
}

func (s *ipv4Set) add(ip uint32) {
	w := ip >> 6                // ip / 64
	b := uint64(1) << (ip & 63) // ip % 64
	s.bitmap[w] |= b
}

func (s *ipv4Set) count() uint64 {
	var total uint64
	for _, w := range s.bitmap {
		total += uint64(bits.OnesCount64(w))
	}
	return total
}
