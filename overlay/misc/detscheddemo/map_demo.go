package main

import (
	"flag"
	"fmt"
	"runtime"
)

func main() {
	entries := flag.Int("entries", 50000, "number of map entries")
	repeats := flag.Int("repeats", 5, "number of full map iterations")
	procs := flag.Int("procs", 1, "GOMAXPROCS")
	flag.Parse()

	runtime.GOMAXPROCS(*procs)

	h := runMapIterHash(*entries, *repeats)
	fmt.Printf("%016x\n", h)
}

func runMapIterHash(entries, repeats int) uint64 {
	m := make(map[uint64]uint64, entries)
	for i := 0; i < entries; i++ {
		k := splitmix64(uint64(i + 1))
		v := splitmix64(k ^ 0x9e3779b97f4a7c15)
		m[k] = v
	}

	h := uint64(0xcbf29ce484222325)
	for r := 0; r < repeats; r++ {
		for k, v := range m {
			x := k ^ rotl(v, int((k%63)+1)) ^ uint64(r+1)
			x ^= x >> 30
			x *= 0xbf58476d1ce4e5b9
			x ^= x >> 27
			x *= 0x94d049bb133111eb
			x ^= x >> 31
			h ^= x
			h *= 0x100000001b3
		}
	}
	return h
}

func splitmix64(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return x
}

func rotl(x uint64, k int) uint64 {
	return (x << k) | (x >> (64 - k))
}
