package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	rttrace "runtime/trace"
	"strings"
)

func main() {
	workers := flag.Int("workers", 64, "number of worker goroutines")
	iters := flag.Int("iters", 2000, "iterations per worker")
	procs := flag.Int("procs", 1, "GOMAXPROCS")
	dumpOrder := flag.Bool("dump-order", false, "print receive-order transcript")
	flag.Parse()

	requireDetSched()
	runtime.GOMAXPROCS(*procs)
	h, order := run(*workers, *iters)
	fmt.Printf("%016x\n", h)
	if *dumpOrder {
		for i, id := range order {
			if i > 0 {
				fmt.Print(",")
			}
			fmt.Print(id)
		}
		fmt.Println()
	}
}

func requireDetSched() {
	err := rttrace.Start(io.Discard)
	if err == nil {
		rttrace.Stop()
		fmt.Fprintln(os.Stderr, "deterministic scheduler mode is not active")
		os.Exit(2)
	}
	if !strings.Contains(err.Error(), "disabled in deterministic scheduler mode") {
		fmt.Fprintf(os.Stderr, "unexpected trace start error: %v\n", err)
		os.Exit(2)
	}
}

func run(workers, iters int) (uint64, []uint64) {
	type result struct {
		id uint64
		v  uint64
	}

	out := make(chan result, workers)
	ready := make(chan struct{}, workers)
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		id := uint64(i + 1)
		go func() {
			ready <- struct{}{}
			<-start
			h := uint64(0x9e3779b97f4a7c15 ^ id)
			local := make(map[uint64]uint64, 32)
			for j := 0; j < iters; j++ {
				a := mix(h + uint64(j) + id)
				b := mix(a + uint64(j<<1))
				local[a&31] = b ^ id

				// Select order is part of patch coverage.
				c1 := make(chan uint64, 1)
				c2 := make(chan uint64, 1)
				c1 <- a
				c2 <- b
				select {
				case x := <-c1:
					<-c2
					h ^= mix(x + 0xa5a5a5a5a5a5a5a5)
				case x := <-c2:
					<-c1
					h ^= mix(x + 0x5a5a5a5a5a5a5a5a)
				}
				runtime.Gosched()
			}
			for k, v := range local {
				h ^= mix(k ^ v)
			}
			out <- result{id: id, v: h}
		}()
	}

	for i := 0; i < workers; i++ {
		<-ready
	}
	close(start)
	hash := uint64(0xcbf29ce484222325)
	order := make([]uint64, 0, workers)
	for i := 0; i < workers; i++ {
		r := <-out
		order = append(order, r.id)
		hash ^= mix(r.v ^ r.id*0x100000001b3)
		hash = (hash << 13) | (hash >> 51)
	}
	return hash, order
}

func mix(x uint64) uint64 {
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return x
}
