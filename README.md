# go-detsched

Deterministic scheduler patch distribution for Go `1.26.0`.

This repo distributes a patch and simple scripts so you can build a patched Go toolchain without maintaining a full Go fork.

## What this repo contains

- `detsched.git.patch`: compiled apply artifact (git-native patch with file modes)
- `patches/detsched.inline.patch`: inline edits to existing upstream Go files
- `new-files/`: dedicated source tree for files that are entirely new to upstream Go
- `scripts/compile-patch.sh`: compiles inline patch + `new-files/` into `detsched.git.patch`
- `scripts/build.sh`: main script (apply + build + patch tests + optional install)
- `scripts/run-tests.sh`: concise external verification suite for patch behavior
- `demos/`: demo docs/examples that are intentionally outside the Go source tree

## Patch maintenance workflow

When you modify this repo's patch content:

1. Edit existing upstream files in `patches/detsched.inline.patch`.
2. Put any entirely new upstream files under `new-files/` with upstream-relative paths.
3. Recompile the final patch artifact:

```bash
./scripts/compile-patch.sh
```

4. Commit all related changes, including the regenerated `detsched.git.patch`.

## Quick start

```bash
./scripts/build.sh --go-tag go1.26.0 --prefix "$HOME/.local/go-detsched-1.26.0"
```

Then use it:

```bash
export GOROOT="$HOME/.local/go-detsched-1.26.0"
export PATH="$GOROOT/bin:$PATH"
GODEBUG=detsched=1,detschedseed=12345 go run ./your_program.go
# Deterministic scheduler fuzzer mode (seed-reproducible perturbations)
GODEBUG=detsched=1,detschedfuzz=1,detschedseed=12345 go run ./your_program.go
```

## Common modes

```bash
# Build + run patch tests, but do not install into --prefix
./scripts/build.sh --go-tag go1.26.0 --no-install

# Build only (skip patch tests)
./scripts/build.sh --go-tag go1.26.0 --no-verify
```

By default, `build.sh` does:

- apply check
- patch apply
- `make.bash`
- `scripts/run-tests.sh` (seed reproducibility, guardrails, fuzz exploration)
- install to `--prefix` (unless `--no-install`)

The patch file is the canonical apply artifact for consumers.

## Compatibility

| Detsched Repo Tag | Upstream Go Tag | Status |
|---|---|---|
| (unreleased) | `go1.26.0` | Verified: patch apply + build + `run-tests.sh` |

Verification contract:

1. `git apply --check` succeeds.
2. Patch applies cleanly.
3. `src/make.bash` succeeds.
4. `scripts/run-tests.sh` succeeds against the patched toolchain.

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
- `detschedfuzz=1`
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
   - adds `detsched`, `detschedfuzz`, and `detschedseed`
3. **Scheduler integration** in `src/runtime/proc.go`
   - init hook
   - startup `GOMAXPROCS` forcing
   - sysmon gating through detsched policy
   - deterministic runq randomization hooks and fuzz perturbations
4. **Select deterministic permutation** in `src/runtime/select.go`
5. **Deterministic random roots** in `src/runtime/rand.go` and `src/runtime/alg.go`
   - stable RNG seed path
   - stable hash key / AES hash schedule initialization
6. **Map internals** in `src/internal/runtime/maps/*`
   - deterministic seed and iterator offset helpers
7. **Trace/profile guardrails**
   - `src/runtime/trace.go`
   - `src/runtime/cpuprof.go`

### Test coverage

Patch verification lives outside the Go source tree in `tests/` and is run by
`scripts/run-tests.sh`. The suite is intentionally concise and verifies:

- same seed => same workload hash
- different seed => different workload hash
- deterministic-mode guardrails (startup `GOMAXPROCS=1`, trace blocked)
- fuzz mode explores schedule space (bounded scan contains both pass and fail)

### Demos

Human-oriented demos are in `demos/` and can be moved to another repo without
changing patch compilation or CI test flow.

### Applying patch manually

From a clean upstream Go source root:

```bash
git apply /path/to/detsched.git.patch
cd src && ./make.bash
```

## Notes

- This is a patch-distribution repo, not a full Go source mirror.
- External nondeterministic inputs (signals, external I/O, etc.) are still out of scope for determinism guarantees.
