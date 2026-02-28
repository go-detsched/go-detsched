package main

import (
	"flag"
	"fmt"
	"runtime"
)

func main() {
	iters := flag.Int("iters", 200000, "number of select iterations")
	procs := flag.Int("procs", 1, "GOMAXPROCS")
	flag.Parse()

	runtime.GOMAXPROCS(*procs)

	h := runSelectHash(*iters)
	fmt.Printf("%016x\n", h)
}

func runSelectHash(iters int) uint64 {
	c1 := make(chan int, 1)
	c2 := make(chan int, 1)
	c3 := make(chan int, 1)
	c4 := make(chan int, 1)
	c5 := make(chan int, 1)

	var h uint64 = 0xcbf29ce484222325
	for i := 0; i < iters; i++ {
		// Make all branches ready at the same time.
		c1 <- i
		c2 <- i ^ 0x55
		c3 <- i ^ 0xaa
		c4 <- i ^ 0x33
		c5 <- i ^ 0xcc

		var branch uint64
		var v int
		select {
		case v = <-c1:
			branch = 1
		case v = <-c2:
			branch = 2
		case v = <-c3:
			branch = 3
		case v = <-c4:
			branch = 4
		case v = <-c5:
			branch = 5
		}

		// Drain the remaining ready channels for next iteration.
		select {
		case <-c1:
		default:
		}
		select {
		case <-c2:
		default:
		}
		select {
		case <-c3:
		default:
		}
		select {
		case <-c4:
		default:
		}
		select {
		case <-c5:
		default:
		}

		// Hash branch + value sequence.
		x := (uint64(v) * 0x517cc1b727220a95) ^ (branch * 0x9e3779b97f4a7c15) ^ (uint64(i) * 0x94d049bb133111eb)
		x ^= x >> 30
		x *= 0xbf58476d1ce4e5b9
		x ^= x >> 27
		x *= 0x94d049bb133111eb
		x ^= x >> 31
		h ^= x
		h *= 0x100000001b3
	}
	return h
}
