package main

import (
	"flag"
	"fmt"
	"runtime"
	"sync"
	"time"
)

func main() {
	workers := flag.Int("workers", 64, "number of timer workers")
	rounds := flag.Int("rounds", 2000, "timer rounds per worker")
	procs := flag.Int("procs", 1, "GOMAXPROCS")
	flag.Parse()

	runtime.GOMAXPROCS(*procs)
	h := runTimerWorkload(*workers, *rounds)
	fmt.Printf("%016x\n", h)
}

func runTimerWorkload(workers, rounds int) uint64 {
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
			acc := uint64(0x510e527fade682d1 ^ id)
			for i := 0; i < rounds; i++ {
				<-time.After(time.Nanosecond)
				acc ^= (uint64(i+1) * 0x9e3779b97f4a7c15) ^ id
				acc = (acc << 11) | (acc >> 53)
				acc *= 0x94d049bb133111eb
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

	h := uint64(0xcbf29ce484222325)
	for i := 1; i <= workers; i++ {
		h ^= results[i]
		h *= 0x100000001b3
	}
	return h
}
