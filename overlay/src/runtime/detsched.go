// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"internal/runtime/atomic"
	_ "unsafe"
)

var detsched struct {
	enabled atomic.Bool
	seed    uint64
}

const (
	detschedSaltMix          = 0x9e3779b97f4a7c15
	detschedSaltRunNext      = 0x72756e6e65787400
	detschedSaltRunQPutSlow  = 0x72716d6978000001
	detschedSaltRunQPutBatch = 0x72716d6978000002
	detschedSaltSelect       = 0x73656c6563740000
	detschedSaltRandInit     = 0x72616e6400000000
	detschedSaltHashKey      = 0x6861736800000000
	detschedSaltAESKey       = 0x6165736800000000
)

func detschedInit() {
	if debug.detsched == 0 && debug.detschedseed == 0 {
		return
	}
	seed := uint64(debug.detschedseed)
	if seed == 0 {
		seed = 1
	}
	detsched.seed = seed
	detsched.enabled.Store(true)

	// Deterministic mode always disables async preemption and
	// adaptive GOMAXPROCS background updates. It also disables
	// runtime tracing and memory profiling to avoid background noise.
	debug.asyncpreemptoff = 1
	debug.updatemaxprocs = 0
	debug.traceallocfree.Store(0)
	MemProfileRate = 0
	detschedForceTimerChanSync()
}

func detschedEnabled() bool {
	return detsched.enabled.Load()
}

func detschedRequested() bool {
	return detschedEnabled() || debug.detsched != 0 || debug.detschedseed != 0
}

func detschedForceTimerChanSync() {
	if detschedRequested() {
		debug.asynctimerchan.Store(0)
	}
}

func sysmonEnabled() bool {
	return haveSysmon && !detschedEnabled()
}

func detschedTraceAllowed() bool {
	return !detschedEnabled()
}

func detschedCPUProfileAllowed(hz int) bool {
	return !detschedEnabled() || hz <= 0
}

func schedulerRandomized() bool {
	return raceenabled || detschedEnabled()
}

func detschedForceStartupProcs(procs int32) int32 {
	if detschedEnabled() {
		return 1
	}
	return procs
}

func schedulerRandnFrom(n uint32, salt uint64) uint32 {
	if n <= 1 {
		return 0
	}
	if !detschedEnabled() {
		return cheaprandn(n)
	}
	x := detsched.seed ^ (salt + detschedSaltMix)
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return uint32(x % uint64(n))
}

func schedulerDropRunNext(goid uint64) bool {
	return schedulerRandomized() && schedulerRandnFrom(2, detschedSaltRunNext^goid) == 0
}

func schedulerShuffleIndexRunQPutSlow(i uint32, goid uint64) uint32 {
	return schedulerRandnFrom(i+1, detschedSaltRunQPutSlow^uint64(i)^goid)
}

func schedulerShuffleIndexRunQPutBatch(i uint32, goid uint64) uint32 {
	return schedulerRandnFrom(i+1, detschedSaltRunQPutBatch^uint64(i)^goid)
}

func schedulerSelectPermuteIndex(i, norder int) uint32 {
	return schedulerRandnFrom(uint32(norder+1), detschedSaltSelect^uint64(i)^uint64(norder))
}

func detschedRand64(salt uint64) uint64 {
	x := detsched.seed ^ (salt + detschedSaltMix)
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return x
}

func detschedRandInitWord(i int) uint64 {
	return detschedRand64(detschedSaltRandInit | uint64(i))
}

func detschedHashKeyWord(i int) uint64 {
	return detschedRand64(detschedSaltHashKey | uint64(i))
}

func detschedAESKeyWord(i int) uint64 {
	return detschedRand64(detschedSaltAESKey | uint64(i))
}

//go:linkname detschedMapRand runtime.detschedMapRand
func detschedMapRand(salt uintptr) uint64 {
	if !detschedEnabled() {
		return rand()
	}
	return detschedRand64(uint64(salt))
}
