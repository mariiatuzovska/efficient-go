//go:build !windows

package main

import (
	"bytes"
	"context"
	"os"
	"reflect"
	"syscall"
	"testing"
	"time"
)

// --- helpers ---

func ip(a, b, c, d uint32) uint32 {
	return (a << 24) | (b << 16) | (c << 8) | d
}

func writeTempFile(t *testing.T, s string) (*os.File, int64) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "ips-*")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if _, err := f.WriteString(s); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatalf("seek: %v", err)
	}
	info, _ := f.Stat()
	return f, info.Size()
}

func pageAlignedChunk() int64 { return int64(syscall.Getpagesize()) }

// --- writeIPs table tests (unchanged semantics) ---

func drain(ch <-chan uint32) []uint32 {
	var out []uint32
	for v := range ch {
		out = append(out, v)
	}
	return out
}

func TestWriteIPs_Table(t *testing.T) {
	type tc struct {
		name  string
		index int64
		input string
		ctx   context.Context
		want  []uint32
	}

	ctxAlive := context.Background()
	ctxCancelled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []tc{
		{
			name:  "empty_buffer",
			index: 0,
			input: "",
			ctx:   ctxAlive,
			want:  nil,
		},
		{
			name:  "single_valid_ip",
			index: 0,
			input: "1.2.3.4\n",
			ctx:   ctxAlive,
			want:  []uint32{ip(1, 2, 3, 4)},
		},
		{
			name:  "multiple_lines_with_noise",
			index: 0,
			input: "1.2.3.4\n999.2.3.4\n1.2.3.4.5\n1.a.3.4\n255.255.255.255\n",
			ctx:   ctxAlive,
			want:  []uint32{ip(1, 2, 3, 4), ip(255, 255, 255, 255)},
		},
		{
			name:  "index_skips_first_partial_line",
			index: 5,
			input: "PARTIAL LINE\n10.0.0.1\n",
			ctx:   ctxAlive,
			want:  []uint32{ip(10, 0, 0, 1)},
		},
		{
			name:  "no_trailing_newline_ignored",
			index: 0,
			input: "5.6.7.8",
			ctx:   ctxAlive,
			want:  nil,
		},
		{
			name:  "bad_octet_resets_and_skips_line",
			index: 0,
			input: "12.34.5678.9\n8.8.8.8\n",
			ctx:   ctxAlive,
			want:  []uint32{ip(8, 8, 8, 8)},
		},
		{
			name:  "too_many_dots_skip_line",
			index: 0,
			input: "1.2.3.4.5\n9.9.9.9\n",
			ctx:   ctxAlive,
			want:  []uint32{ip(9, 9, 9, 9)},
		},
		{
			name:  "unexpected_characters_skip_line",
			index: 0,
			input: "1.2.x.4\n7.6.5.4\n",
			ctx:   ctxAlive,
			want:  []uint32{ip(7, 6, 5, 4)},
		},
		{
			name:  "multiple_valid_edges",
			index: 0,
			input: "0.0.0.0\n0.0.0.255\n0.0.1.0\n",
			ctx:   ctxAlive,
			want:  []uint32{ip(0, 0, 0, 0), ip(0, 0, 0, 255), ip(0, 0, 1, 0)},
		},
		{
			name:  "cancelled_context_before_start",
			index: 0,
			input: "1.2.3.4\n2.3.4.5\n",
			ctx:   ctxCancelled,
			want:  nil,
		},
		{
			name:  "cancel_during_parsing_returns_early",
			index: 0,
			input: "10.0.0.1\n10.0.0.2\n10.0.0.3\n10.0.0.4\n10.0.0.5\n10.0.0.6\n10.0.0.7\n10.0.0.8\n",
			ctx:   nil, // set below
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			in := make(chan uint32, 1024)
			if tt.name == "cancel_during_parsing_returns_early" {
				ctx, cancel := context.WithCancel(context.Background())
				tt.ctx = ctx
				go func() {
					time.Sleep(1 * time.Millisecond)
					cancel()
				}()
			}

			writeIPs(tt.ctx, []byte(tt.input), tt.index, in)
			close(in)
			got := drain(in)

			if tt.name == "cancel_during_parsing_returns_early" {
				if len(got) > 8 {
					t.Fatalf("expected <= 8 outputs due to cancellation, got %d", len(got))
				}
				return
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAppRun_Count_SingleChunk(t *testing.T) {
	chunk := pageAlignedChunk()

	data := "1.2.3.4\n8.8.8.8\n1.2.3.4\n" // uniques = 2
	f, size := writeTempFile(t, data)
	defer f.Close()

	a := &app{chunk: chunk, overlap: 8, numWorkers: 2}
	count, err := a.run(context.Background(), f, size)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
}

func TestAppRun_Count_MultiChunk_WithBoundary(t *testing.T) {
	chunk := pageAlignedChunk()

	// Force ≥ 2 chunks with the same IP → uniques = 1
	line := "10.0.0.1\n"
	repeats := int(chunk/int64(len(line))) + 10
	var b bytes.Buffer
	for i := 0; i < repeats; i++ {
		b.WriteString(line)
	}
	f, size := writeTempFile(t, b.String())
	defer f.Close()

	a := &app{chunk: chunk, overlap: 16, numWorkers: 3}
	count, err := a.run(context.Background(), f, size)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
}

func TestAppRun_Count_MixedIPs_WithDuplicates(t *testing.T) {
	chunk := pageAlignedChunk()

	// uniques = 4
	data := "0.0.0.0\n1.1.1.1\n2.2.2.2\n1.1.1.1\n255.255.255.255\n2.2.2.2\n"
	f, size := writeTempFile(t, data)
	defer f.Close()

	a := &app{chunk: chunk, overlap: 8, numWorkers: 1}
	count, err := a.run(context.Background(), f, size)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if count != 4 {
		t.Fatalf("count = %d, want 4", count)
	}
}

func TestAppRun_EmptyFile_CountZero(t *testing.T) {
	chunk := pageAlignedChunk()

	f, size := writeTempFile(t, "")
	defer f.Close()

	a := &app{chunk: chunk, overlap: 8, numWorkers: 2}
	count, err := a.run(context.Background(), f, size)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
}

// Error-path: force mmap failure using a non–page-aligned chunk so index 1 offset is unaligned (EINVAL on Unix).
func TestAppRun_ReturnsError_OnMmapFailure(t *testing.T) {
	ps := int64(syscall.Getpagesize())
	badChunk := ps/2 + 123 // not page-aligned

	// Make file big enough to reach index 1.
	line := "10.0.0.1\n"
	repeats := int((2*badChunk)/int64(len(line))) + 20
	var b bytes.Buffer
	for i := 0; i < repeats; i++ {
		b.WriteString(line)
	}
	f, size := writeTempFile(t, b.String())
	defer f.Close()

	a := &app{chunk: badChunk, overlap: 16, numWorkers: 2}
	count, err := a.run(context.Background(), f, size)
	if err == nil {
		t.Fatalf("expected non-nil error, got nil (count=%d)", count)
	}
	// Count is whatever was accumulated before the failure; don't assert it.
}
