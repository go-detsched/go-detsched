# Deterministic Scheduler (`detsched`) Feature

This document explains what was implemented for the deterministic scheduler feature, why each change was made, and how to use and maintain it.

It describes the standalone patch:

- `detsched-only-feature.git.patch`

located in this same directory.

## Goal

Provide an opt-in runtime mode for in-process deterministic behavior:

- same seed => stable execution outcomes
- no WASM dependency
- scoped to in-process concurrency/state (goroutines, channels, select, timers, map iteration/randomized hashing)

The core design is a seeded deterministic policy in the runtime that replaces schedule/random choices normally driven by runtime randomness.

## Non-goals / Scope boundaries

Not intended to deterministically model:

- external I/O side effects
- OS scheduling and signal timing as externally observable behavior
- full record/replay

This feature is deterministic scheduling/randomization mode, not event-log replay.

## User-facing controls

The feature is enabled via `GODEBUG`:

- `detsched=1` enables deterministic mode
- `detschedseed=<n>` sets deterministic seed

Behavior defaults in deterministic mode:

- async preemption disabled
- adaptive `GOMAXPROCS` background updates disabled
- startup `GOMAXPROCS` forced to `1`
- runtime tracing blocked
- CPU profiling blocked
- memory profiling rate forced to `0`

## Why these controls are needed

Scheduler decisions are only one source of nondeterminism. Even with a deterministic scheduler policy, background runtime behavior can perturb execution order indirectly. The mode therefore disables or constrains major background/noise sources that can change execution timing and control flow.

## High-level implementation

### 1) Central deterministic policy module

Added `src/runtime/detsched.go` as the policy center:

- stores enabled flag + seed
- initializes mode from `debug.detsched` / `debug.detschedseed`
- centralizes deterministic salt constants
- provides small helper APIs used by call sites

Key helpers include:

- `detschedEnabled`
- `sysmonEnabled`
- `detschedTraceAllowed`
- `detschedCPUProfileAllowed`
- `schedulerDropRunNext`
- `schedulerShuffleIndexRunQPutSlow`
- `schedulerShuffleIndexRunQPutBatch`
- `schedulerSelectPermuteIndex`
- `detschedRandInitWord`
- `detschedHashKeyWord`
- `detschedAESKeyWord`

This isolation keeps policy logic in one file and leaves call sites thin.

### 2) Runtime debug wiring

Updated `src/runtime/runtime1.go`:

- added debug fields:
  - `detsched`
  - `detschedseed`
- registered both in `dbgvars` for `GODEBUG` parsing

### 3) Scheduler integration (`proc.go`)

Updated `src/runtime/proc.go`:

- call `detschedInit()` in runtime init path
- apply `detschedForceStartupProcs` during startup `GOMAXPROCS` selection
- use `sysmonEnabled()` when deciding whether to spawn sysmon
- use deterministic helper APIs for run queue randomized choices

Rationale:

- deterministic schedule selection must replace runtime randomization points
- single-P startup and sysmon gating reduce nondeterministic interference

### 4) `select` deterministic branch permutation

Updated `src/runtime/select.go`:

- replaced select-case shuffle random source with `schedulerSelectPermuteIndex`

Rationale:

- multiple-ready `select` is a classic source of nondeterminism

### 5) Deterministic runtime random roots (hash/map/select consistency)

Updated:

- `src/runtime/rand.go`
- `src/runtime/alg.go`

Changes:

- deterministic initialization of global runtime RNG seed in mode
- deterministic initialization of hash keys and AES hash schedule keys in mode

Rationale:

- many runtime and map behaviors depend on these random roots
- stable seeds are required for stable outcomes across runs

### 6) Map runtime internals

Updated:

- `src/internal/runtime/maps/runtime.go`
- `src/internal/runtime/maps/map.go`
- `src/internal/runtime/maps/table.go`

Changes:

- added map-specific helper wrappers for seeding and iterator offsets
- map seed initialization and reseeding use these wrappers
- iterator entry/directory offsets use deterministic map helpers

Rationale:

- map hash seed + iterator offsets are significant nondeterminism sources

### 7) Tracing/profiling guards

Updated:

- `src/runtime/trace.go`
- `src/runtime/cpuprof.go`
- plus detsched init policy in `src/runtime/detsched.go`

Changes:

- `StartTrace` refuses when deterministic mode is active
- `SetCPUProfileRate(hz>0)` refuses when deterministic mode is active
- `traceallocfree` forced off in mode
- `MemProfileRate` set to `0` in mode

Rationale:

- tracing/profiling can add background activity and timing perturbations

## Userspace demos and validation assets

Added `misc/detscheddemo` programs/scripts:

- arithmetic workload demo (`main.go`)
- `select` multi-ready demo (`select_demo.go`)
- map iteration demo (`map_demo.go`)
- GC pressure demo (`gc_demo.go`)
- timer demo (`timer_demo.go`)
- high-intensity combined stress demo (`stress_demo.go`) that runs hundreds of goroutines and over a million `Gosched` yields while exercising select/map/timer/alloc/GC paths together

Runner scripts:

- `run_seed_demo.sh`
- `run_select_demo.sh`
- `run_map_demo.sh`
- `run_gc_demo.sh`
- `run_timer_demo.sh`
- `run_stress_demo.sh`
- `run_synctest_demo.sh`
- `run_all_demos.sh`

Added synctest example:

- `src/testing/synctest/detsched_demo_test.go`

## Build and test flow used

1. Build patched toolchain with `src/make.bash`
2. Run deterministic demo scripts
3. Verify same-seed stability and (where expected) changed behavior for different seeds

Notes:

- some workloads may legitimately produce same hash for different seeds (scripts print a warning, not always failure)
- same-seed instability is considered failure

## Isolation and maintainability decisions

To make rebases easier, we refactored to:

- centralize salts/policy in `detsched.go`
- keep hot-file edits (`proc.go`, `select.go`, maps) as helper calls
- avoid scattering magic constants across runtime files
- keep map-specific salt policy in map runtime wrapper helpers

This is specifically meant to reduce merge pain when upstream scheduler/map internals evolve.

## Known limitations / trade-offs

- This is not full deterministic replay.
- It intentionally changes runtime behavior under mode (single-P startup and guardrails).
- Determinism target is in-process behavior; external side effects remain out of scope.

## Applying the patch

From a clean upstream Go source tree root:

- `git apply /path/to/detsched-only-feature.git.patch`

Then build:

- `cd src && ./make.bash`
