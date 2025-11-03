//go:build !windows

package main

import (
	"os"
	"syscall"
	"testing"
)

func mustWriteTemp(t *testing.T, size int) (*os.File, []byte) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "mmap-test-*")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	buf := make([]byte, size)
	for i := 0; i < size; i++ {
		buf[i] = byte(i)
	}
	if _, err := f.Write(buf); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatalf("seek: %v", err)
	}
	return f, buf
}

func unmapIfMapped(t *testing.T, b []byte) {
	t.Helper()
	if b != nil {
		if err := syscall.Munmap(b); err != nil {
			t.Fatalf("Munmap: %v", err)
		}
	}
}

func TestOpenFile_OKAndSize(t *testing.T) {
	const size = 90
	f, want := mustWriteTemp(t, size)
	defer f.Close()

	gotF, gotSize, err := openFile(f.Name())
	if err != nil {
		t.Fatalf("openFile: %v", err)
	}
	defer gotF.Close()

	if gotSize != int64(size) {
		t.Fatalf("size = %d, want %d", gotSize, size)
	}
	fi, _ := gotF.Stat()
	if fi.Size() != int64(len(want)) {
		t.Fatalf("stat size mismatch: %d vs %d", fi.Size(), len(want))
	}
}

func TestOpenFile_NotExist(t *testing.T) {
	_, _, err := openFile("this-definitely-does-not-exist.txt")
	if err == nil {
		t.Fatalf("expected error for non-existent file")
	}
}

func TestMemoryMap_FirstChunk_WithOverlap_PageAligned(t *testing.T) {
	ps := syscall.Getpagesize()
	// Choose sizes so that chunk and offsets are page aligned.
	chunk := int64(2 * ps) // page-aligned
	overlap := int64(4)    // small overlap
	fileSize := 5*ps + 10  // not multiple of chunk to exercise overlap/last math

	f, want := mustWriteTemp(t, fileSize)
	defer f.Close()

	mm := memoryMap{f: f, size: int64(fileSize), chunk: chunk, overlap: overlap}

	if err := mm.mmap(0); err != nil {
		t.Fatalf("mmap: %v", err)
	}
	defer unmapIfMapped(t, mm.b)

	wantLen := int(chunk + overlap)
	if len(mm.b) != wantLen {
		t.Fatalf("len(b) = %d, want %d", len(mm.b), wantLen)
	}
	if string(mm.b) != string(want[:wantLen]) {
		t.Fatalf("mapped bytes mismatch for first chunk")
	}
}

func TestMemoryMap_MiddleChunk_WithOverlap_PageAligned(t *testing.T) {
	ps := syscall.Getpagesize()
	chunk := int64(2 * ps) // page-aligned chunk
	overlap := int64(4)
	fileSize := 5*ps + 10

	f, want := mustWriteTemp(t, fileSize)
	defer f.Close()

	mm := memoryMap{f: f, size: int64(fileSize), chunk: chunk, overlap: overlap}

	// index 1 -> offset = chunk (aligned), length = chunk + overlap
	if err := mm.mmap(1); err != nil {
		t.Fatalf("mmap: %v", err)
	}
	defer unmapIfMapped(t, mm.b)

	offset := int(chunk * 1)
	wantLen := int(chunk + overlap)
	if len(mm.b) != wantLen {
		t.Fatalf("len(b) = %d, want %d", len(mm.b), wantLen)
	}
	if string(mm.b) != string(want[offset:offset+wantLen]) {
		t.Fatalf("mapped bytes mismatch for middle chunk")
	}
}

func TestMemoryMap_LastChunk_ShorterThanChunkPlusOverlap_PageAligned(t *testing.T) {
	ps := syscall.Getpagesize()
	chunk := int64(2 * ps) // page-aligned
	overlap := int64(4)
	fileSize := 5*ps + 10

	f, want := mustWriteTemp(t, fileSize)
	defer f.Close()

	mm := memoryMap{f: f, size: int64(fileSize), chunk: chunk, overlap: overlap}

	// index 2 -> offset = 2*chunk = 4*ps (aligned)
	// left = fileSize - offset = ps + 10 < chunk + overlap
	if err := mm.mmap(2); err != nil {
		t.Fatalf("mmap: %v", err)
	}
	defer unmapIfMapped(t, mm.b)

	offset := int(chunk * 2)
	left := int(mm.size - int64(offset))
	if len(mm.b) != left {
		t.Fatalf("len(b) = %d, want %d (left bytes)", len(mm.b), left)
	}
	if string(mm.b) != string(want[offset:offset+left]) {
		t.Fatalf("mapped bytes mismatch for last short chunk")
	}
}

func TestMemoryMap_OffsetBeyondEnd_NoMapAndNoError(t *testing.T) {
	ps := syscall.Getpagesize()
	chunk := int64(2 * ps)
	overlap := int64(4)
	fileSize := 2 * ps // small

	f, _ := mustWriteTemp(t, fileSize)
	defer f.Close()

	mm := memoryMap{f: f, size: int64(fileSize), chunk: chunk, overlap: overlap}

	// index 2 -> offset = 4*ps > size -> returns nil, no map
	if err := mm.mmap(2); err != nil {
		t.Fatalf("mmap: unexpected error: %v", err)
	}
	if mm.b != nil {
		t.Fatalf("expected no mapping when offset beyond file; got len=%d", len(mm.b))
	}
}
