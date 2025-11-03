package main

import (
	"os"
	"syscall"
)

type memoryMap struct {
	f              *os.File
	size           int64
	chunk, overlap int64
	b              []byte
}

func openFile(path string) (f *os.File, size int64, _ error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}

	st, err := f.Stat()
	if err != nil {
		return nil, 0, err
	}

	return f, st.Size(), nil
}

func (mm *memoryMap) mmap(index int64) (err error) {
	offset := index * mm.chunk
	length := mm.chunk + mm.overlap
	if left := mm.size - offset; left < 1 {
		return nil
	} else if left < length {
		length = left
	}
	// syscal will block 1 thread
	mm.b, err = syscall.Mmap(
		int(mm.f.Fd()), offset, int(length), syscall.PROT_READ, syscall.MAP_SHARED,
	)
	return err
}
