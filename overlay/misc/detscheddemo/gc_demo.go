package main

import (
	"flag"
	"fmt"
	"runtime"
	"sync"
)

func main() {
	workers := flag.Int("workers", 32, "number of workers")
	rounds := flag.Int("rounds", 4000, "allocation rounds per worker")
	procs := flag.Int("procs", 1, "GOMAXPROCS")
	flag.Parse()

	runtime.GOMAXPROCS(*procs)
	h := runGCWorkload(*workers, *rounds)
	fmt.Printf("%016x\n", h)
}

func runGCWorkload(workers, rounds int) uint64 {
	type result struct {
		id  int
		acc uint64
	}
	out := make(chan result, workers)
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		id := uint64(w + 1)
		go func() {
			defer wg.Done()
			acc := uint64(0x6a09e667f3bcc909 ^ id)
			for i := 0; i < rounds; i++ {
				n := int((id+uint64(i))%128 + 256)
				buf := make([]byte, n)
				for j := range buf {
					buf[j] = byte((int(id) + i + j) & 0xff)
				}
				for _, b := range buf {
					acc ^= uint64(b) + 0x9e3779b97f4a7c15
					acc = (acc << 7) | (acc >> 57)
					acc *= 0xbf58476d1ce4e5b9
				}
				if i&255 == 0 {
					runtime.GC()
				}
			}
			out <- result{id: int(id), acc: acc}
		}()
	}
	go func() {
		wg.Wait()
		close(out)
	}()

	results := make([]uint64, workers+1)
	for r := range out {
		results[r.id] = r.acc
	}

	hash := uint64(0xcbf29ce484222325)
	for i := 1; i <= workers; i++ {
		hash ^= results[i]
		hash *= 0x100000001b3
	}
	return hash
}
