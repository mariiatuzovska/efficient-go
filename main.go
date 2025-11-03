package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	// _ "net/http/pprof"
	"runtime"
)

const MB = 1024 * 1024

var (
	chank      = flag.Int64("c", 32*MB, "number of bytes to be read into memory by worker")
	overlap    = flag.Int64("o", 14, "number of bytes to be added to the chank per read for correcting line(s)")
	numWorkers = flag.Int("n", runtime.GOMAXPROCS(-1)-1, "number of parallel workers")
	verbose    = flag.Bool("v", false, "verbose mode")
)

func main() {
	flag.Parse()
	if flag.NArg() < 1 {
		log.Fatal("file path is required")
	}
	filePath := flag.Arg(0)
	print("reading file: %s", filePath)

	if *chank < 1 || *overlap < 0 || *numWorkers < 1 {
		log.Fatal("incorrect flags")
	}

	f, size, err := openFile(filePath)
	if err != nil {
		log.Fatal("cannot open file")
	}
	defer f.Close()

	print("opened %d MB file", size/MB)
	if size == 0 {
		return
	}

	app := &app{
		chunk:      *chank,
		overlap:    *overlap,
		numWorkers: *numWorkers,
	}

	total, err := app.run(context.Background(), f, size)
	fmt.Println("\nTotal IPs count:", total)
	if err != nil {
		f.Close() // fatal will not call defer f.Close()
		log.Fatal(err)
	}
}

func print(format string, args ...any) {
	if !*verbose {
		return
	}
	fmt.Printf(format+"\n", args...)
}
