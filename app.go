package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

const (
	newLine  = byte('\n')
	zeroChar = byte('0')
	neufChar = byte('9')
)

type app struct {
	chunk      int64
	overlap    int64
	numWorkers int
}

func (app *app) run(ctx context.Context, f *os.File, size int64) (uint64, error) {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	indexes := make(chan int64)
	go func() {
		defer close(indexes)
		// number of chunks, rounding up
		nchunks := (size + (app.chunk) - 1) / (app.chunk)
		for i := int64(0); i < nchunks; i++ {
			select {
			case <-ctx.Done():
				return
			case indexes <- i:
			}
		}
	}()

	ips := make(chan uint32, app.numWorkers)
	ipSet := newIPv4Set()

	errs := make(chan error, app.numWorkers)
	for id := range app.numWorkers {
		go func() {
			var err error
			defer func() { errs <- err }()
			print("starting [%d] worker", id)

			mm := memoryMap{
				f:       f,
				size:    size,
				chunk:   app.chunk,
				overlap: app.overlap,
			}

			for {
				select {
				case <-ctx.Done():
					return
				case i, ok := <-indexes:
					if !ok {
						return // no more work
					}
					if err = mm.mmap(i); err != nil {
						print("failed at %d with error %v", i, err)
						stop() // cancel ctx for all goroutines
						return
					}
					print("[%d] worker: batch %d", id, len(mm.b))
					writeIPs(ctx, mm.b, i, ips)
				}
			}
		}()
	}

	doneWritingIPs := make(chan struct{})
	go func() {
		defer close(doneWritingIPs)
		for ip := range ips {
			ipSet.add(ip)
		}
	}()

	var errArr []error
	for range app.numWorkers {
		if err := <-errs; err != nil {
			errArr = append(errArr, err)
		}
	}

	close(ips)
	<-doneWritingIPs

	if len(errArr) != 0 {
		return ipSet.count(), fmt.Errorf("%v", errArr)
	}
	return ipSet.count(), nil
}

func writeIPs(ctx context.Context, b []byte, index int64, in chan<- uint32) {
	n := len(b)
	if n == 0 {
		return
	}

	var (
		i       int
		a0      uint32
		a1      uint32
		a2      uint32
		a3      uint32
		number  uint32
		segment uint32 // which octet we're filling [0..3]
	)

	// skip first partial line for non-zero chunk index
	// thats why we do overlap while doing `syscall.Mmap` in `mm.mmap(index)`
	if index != 0 {
		for i < n && b[i] != newLine {
			i++
		}
		if i < n {
			i++ // skip new line '\n'
		}
	}

	for i < n {
		select {
		case <-ctx.Done():
			return
		default:
		}

		c := b[i]
		i++

		// parse manually characters to integers
		if c >= zeroChar && c <= neufChar {
			number = number*10 + uint32(c-zeroChar)
			if number > 255 {
				// bad octet
				for i < n && b[i] != newLine {
					i++
				}
				number = 0
				segment = 0
			}
			continue
		}

		if c == '.' {
			switch segment {
			case 0:
				a0 = number
			case 1:
				a1 = number
			case 2:
				a2 = number
			default:
				// too many dots
				for i < n && b[i] != newLine {
					i++
				}
				number = 0
				segment = 0
				continue
			}
			segment++
			number = 0
			continue
		}

		if c == newLine {
			if segment == 3 {
				a3 = number
				ip := (a0 << 24) | (a1 << 16) | (a2 << 8) | a3
				in <- ip
			}
			number = 0
			segment = 0
			continue
		}

		// unexpected char
		for i < n && b[i] != newLine {
			i++
		}
		number = 0
		segment = 0
	}
}
