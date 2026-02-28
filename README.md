# go-detsched

Deterministic scheduler patch distribution for Go `1.26.0`.

This repo distributes a patch and simple scripts so you can build a patched Go toolchain without maintaining a full Go fork.

## What this repo contains

- `detsched.git.patch`: git-native patch (includes file modes)
- `scripts/build.sh`: main script (apply + build + demo verify + optional install)

## Quick start

```bash
./scripts/build.sh --go-tag go1.26.0 --prefix "$HOME/.local/go-detsched-1.26.0"
```

Then use it:

```bash
export GOROOT="$HOME/.local/go-detsched-1.26.0"
export PATH="$GOROOT/bin:$PATH"
GODEBUG=detsched=1,detschedseed=12345 go run ./your_program.go
```

## Common modes

```bash
# Build + run demos, but do not install into --prefix
./scripts/build.sh --go-tag go1.26.0 --no-install

# Build only (skip demo verification)
./scripts/build.sh --go-tag go1.26.0 --no-verify
```

By default, `build.sh` does:

- apply check
- patch apply
- `make.bash`
- `misc/detscheddemo/run_all_demos.sh` (seed + stress + synctest)
- install to `--prefix` (unless `--no-install`)

The patch file is the canonical apply artifact.

## Compatibility

| Detsched Repo Tag | Upstream Go Tag | Status |
|---|---|---|
| (unreleased) | `go1.26.0` | Verified: patch apply + build + `run_all_demos.sh` |

Verification contract:

1. `git apply --check` succeeds.
2. Patch applies cleanly.
3. `src/make.bash` succeeds.
4. `misc/detscheddemo/run_all_demos.sh` succeeds.

## Feature details

### Goal

Provide an opt-in deterministic runtime mode for in-process behavior:

- same seed => stable outcomes
- no WASM dependency
- in-process scope (goroutines, channels, `select`, timers, map hashing/iteration behavior)

This is deterministic scheduling/randomization mode, not record/replay.

### Non-goals

- external I/O determinism
- OS signal timing determinism
- full execution replay

### User controls

Use `GODEBUG`:

- `detsched=1`
- `detschedseed=<n>`

Deterministic-mode defaults:

- async preemption disabled
- adaptive `GOMAXPROCS` updates disabled
- startup `GOMAXPROCS` forced to `1`
- runtime tracing blocked
- CPU profiling blocked
- memory profiling disabled (`MemProfileRate=0`)
- timer channels forced to synchronous mode (`asynctimerchan=0`)

### Runtime changes (high-level)

1. **Policy center** in `src/runtime/detsched.go`
   - enabled flag + seed
   - deterministic salt constants
   - helper APIs for scheduler/select/random policy hooks
2. **Debug plumbing** in `src/runtime/runtime1.go`
   - adds `detsched` and `detschedseed`
3. **Scheduler integration** in `src/runtime/proc.go`
   - init hook
   - startup `GOMAXPROCS` forcing
   - sysmon gating through detsched policy
   - deterministic runq randomization hooks
4. **Select deterministic permutation** in `src/runtime/select.go`
5. **Deterministic random roots** in `src/runtime/rand.go` and `src/runtime/alg.go`
   - stable RNG seed path
   - stable hash key / AES hash schedule initialization
6. **Map internals** in `src/internal/runtime/maps/*`
   - deterministic seed and iterator offset helpers
7. **Trace/profile guardrails**
   - `src/runtime/trace.go`
   - `src/runtime/cpuprof.go`

### Demos maintained

In `misc/detscheddemo`:

- `run_seed_demo.sh`
- `run_stress_demo.sh`
- `run_synctest_demo.sh`
- `run_all_demos.sh`

Stress demo (`stress_demo.go`) is the high-intensity integration test:

- hundreds of goroutines
- more than 1M `runtime.Gosched` yields in default config
- mixed select/map/timer/alloc/GC pressure behavior in one run

### Applying patch manually

From a clean upstream Go source root:

```bash
git apply /path/to/detsched.git.patch
cd src && ./make.bash
```

## Notes

- This is a patch-distribution repo, not a full Go source mirror.
- External nondeterministic inputs (signals, external I/O, etc.) are still out of scope for determinism guarantees.
