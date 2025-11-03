package main

import (
	"testing"
)

// helper: create a small set so tests don't allocate 512MB
func newSmallSet(words int) *ipv4Set {
	return &ipv4Set{bitmap: make([]uint64, words)}
}

func TestIPv4Set(t *testing.T) {
	type tc struct {
		name  string
		words int      // bitmap length in 64-bit words
		ips   []uint32 // IPs to add
		want  uint64   // expected unique count
	}
	tests := []tc{
		{
			name:  "empty",
			words: 1,
			ips:   nil,
			want:  0,
		},
		{
			name:  "single_zero",
			words: 1,
			ips:   []uint32{0},
			want:  1,
		},
		{
			name:  "duplicate_same_ip",
			words: 1,
			ips:   []uint32{0, 0, 0},
			want:  1,
		},
		{
			name:  "two_bits_same_word",
			words: 1,
			ips:   []uint32{1, 63},
			want:  2,
		},
		{
			name:  "cross_word",
			words: 2,
			ips:   []uint32{63, 64},
			want:  2,
		},
		{
			name:  "multiple_with_duplicates",
			words: 2,
			ips:   []uint32{0, 1, 1, 2, 64, 65, 64},
			want:  5,
		},
		{
			name:  "upper_bound_of_small_bitmap",
			words: 2,
			ips:   []uint32{127},
			want:  1,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			s := newSmallSet(tt.words)
			for _, ip := range tt.ips {
				if int(ip>>6) >= tt.words {
					t.Fatalf("test input ip=%d exceeds bitmap words=%d", ip, tt.words)
				}
				s.add(ip)
			}
			if got := s.count(); got != tt.want {
				t.Fatalf("count() = %d, want %d", got, tt.want)
			}
		})
	}
}
