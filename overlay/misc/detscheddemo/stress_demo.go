package main

import (
	"flag"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	workers := flag.Int("workers", 256, "number of worker goroutines")
	iters := flag.Int("iters", 3000, "iterations per worker")
	procs := flag.Int("procs", 1, "GOMAXPROCS")
	flag.Parse()

	runtime.GOMAXPROCS(*procs)
	hash, switches := runStress(*workers, *iters)
	fmt.Printf("hash=%016x switches=%d workers=%d iters=%d\n", hash, switches, *workers, *iters)
}

func runStress(workers, iters int) (uint64, uint64) {
	results := make([]uint64, workers)
	var switches atomic.Uint64
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(workers)

	for w := 0; w < workers; w++ {
		id := w
		go func() {
			defer wg.Done()
			<-start

			workerID := uint64(id + 1)
			state := splitmix64(workerID*0x9e3779b97f4a7c15 + 0x243f6a8885a308d3)
			local := make(map[uint64]uint64, 64)
			aCh := make(chan uint64, 1)
			bCh := make(chan uint64, 1)
			h := uint64(0x6a09e667f3bcc909 ^ workerID)

			for i := 0; i < iters; i++ {
				iter := uint64(i)
				a := next(&state)
				b := next(&state)

				k := (a ^ iter) & 63
				local[k] = b ^ (workerID << 7) ^ iter

				aCh <- a
				bCh <- b
				select {
				case v := <-aCh:
					<-bCh
					h ^= mix64(v ^ 0xa5a5a5a5a5a5a5a5)
				case v := <-bCh:
					<-aCh
					h ^= mix64(v ^ 0x5a5a5a5a5a5a5a5a)
				}

				if i%8 == 0 {
					fold := uint64(0)
					for lk, lv := range local {
						fold ^= mix64(lk*0x9e3779b97f4a7c15 + lv)
					}
					h ^= rotl(fold, int((iter+workerID)%63+1))
				}

				if i%64 == 0 {
					// Exercise timer-channel paths while avoiding wall-clock jitter.
					<-time.After(0)
					h ^= ((a % 3) + 1) << 11
				}

				if id == 0 && i%512 == 0 {
					runtime.GC()
				}

				buf := make([]byte, 64+int(a&63))
				buf[0] = byte(h)
				buf[len(buf)-1] = byte(b)
				h ^= uint64(buf[0]) ^ uint64(buf[len(buf)-1])<<8

				runtime.Gosched()
				switches.Add(1)
				if (a^b)&1 == 0 {
					runtime.Gosched()
					switches.Add(1)
				}
			}

			results[id] = h
		}()
	}

	close(start)
	wg.Wait()

	final := uint64(0xcbf29ce484222325)
	for i, r := range results {
		final ^= mix64(r ^ uint64(i+1)*0x94d049bb133111eb)
		final *= 0x100000001b3
		final = rotl(final, 13)
	}
	return final, switches.Load()
}

func mix64(x uint64) uint64 {
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
	return mix64(*s)
}

func rotl(x uint64, k int) uint64 {
	return (x << k) | (x >> (64 - k))
}
