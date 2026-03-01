package main

import (
	"flag"
	"fmt"
	"runtime"
	"sync"
)

type item struct {
	worker uint64
	iter   uint64
	value  uint64
}

func main() {
	workers := flag.Int("workers", 128, "number of workers")
	iters := flag.Int("iters", 20000, "iterations per worker")
	procs := flag.Int("procs", 1, "GOMAXPROCS")
	flag.Parse()

	runtime.GOMAXPROCS(*procs)
	h := runWorkload(*workers, *iters)
	fmt.Printf("%016x\n", h)
}

func runWorkload(workers, iters int) uint64 {
	out := make(chan item)
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		workerID := uint64(w + 1)
		go func() {
			defer wg.Done()
			state := splitmix64(workerID*0x9e3779b97f4a7c15 + 0x243f6a8885a308d3)
			acc := uint64(0x6a09e667f3bcc909 ^ workerID)
			for i := 0; i < iters; i++ {
				a := next(&state)
				b := next(&state)
				v := arithmetic(a, b, workerID, uint64(i))
				acc ^= rotl(v+uint64(i), int((workerID+uint64(i))%63+1))
				acc *= 0x9e3779b97f4a7c15
			}
			out <- item{worker: workerID, iter: uint64(iters), value: acc}
		}()
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	hash := uint64(0xcbf29ce484222325)
	for it := range out {
		hash ^= mixItem(it)
		hash *= 0x100000001b3
		hash = rotl(hash, 13)
	}
	return hash
}

func arithmetic(a, b, workerID, iter uint64) uint64 {
	x := a*0x9e3779b97f4a7c15 + b
	x ^= rotl(workerID+iter+1, int((workerID+iter)%63+1))
	x = (x ^ (x >> 29)) * 0xbf58476d1ce4e5b9
	x ^= x >> 32
	return x
}

func mixItem(it item) uint64 {
	x := it.value ^ (it.worker * 0x94d049bb133111eb) ^ (it.iter * 0x2545f4914f6cdd1d)
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return x
}

func splitmix64(seed uint64) uint64 {
	return seed
}

func next(s *uint64) uint64 {
	*s += 0x9e3779b97f4a7c15
	z := *s
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

func rotl(x uint64, k int) uint64 {
	return (x << k) | (x >> (64 - k))
}
