// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package synctest_test

import (
	"runtime"
	"testing"
	"testing/synctest"
)

func TestDetSchedSeededBubbleHash(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const workers = 64
		const iters = 1500

		runtime.GOMAXPROCS(1)

		start := make(chan struct{})
		out := make(chan uint64, workers)
		for i := 0; i < workers; i++ {
			workerID := uint64(i + 1)
			go func() {
				<-start
				state := splitmix64(workerID*0x9e3779b97f4a7c15 + 0x243f6a8885a308d3)
				acc := uint64(0x6a09e667f3bcc909 ^ workerID)
				for j := 0; j < iters; j++ {
					a := next(&state)
					b := next(&state)
					v := arithmetic(a, b, workerID, uint64(j))
					acc ^= rotl(v+uint64(j), int((workerID+uint64(j))%63+1))
					acc *= 0x9e3779b97f4a7c15
					if j&15 == 0 {
						runtime.Gosched()
					}
				}
				out <- acc
			}()
		}
		close(start)

		hash := uint64(0xcbf29ce484222325)
		for i := 0; i < workers; i++ {
			v := <-out
			hash ^= mix(v ^ uint64(i+1))
			hash *= 0x100000001b3
		}
		t.Logf("detsched-hash=%016x", hash)
	})
}

func arithmetic(a, b, workerID, iter uint64) uint64 {
	x := a*0x9e3779b97f4a7c15 + b
	x ^= rotl(workerID+iter+1, int((workerID+iter)%63+1))
	x = (x ^ (x >> 29)) * 0xbf58476d1ce4e5b9
	x ^= x >> 32
	return x
}

func mix(x uint64) uint64 {
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
