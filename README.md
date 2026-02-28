# go-detsched

Deterministic scheduler patch distribution for Go `1.26.0`.

This repo distributes a patch and simple scripts so you can build a patched Go toolchain without maintaining a full Go fork.

## What this repo contains

- `detsched-only-feature.git.patch`: git-native patch (includes file modes)
- `DETSCHED_FEATURE.md`: implementation and design notes
- `scripts/build-go-detsched.sh`: main script (apply + build + demo verify + optional install)

## Quick start

```bash
./scripts/build-go-detsched.sh --go-tag go1.26.0 --prefix "$HOME/.local/go-detsched-1.26.0"
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
./scripts/build-go-detsched.sh --go-tag go1.26.0 --no-install

# Build only (skip demo verification)
./scripts/build-go-detsched.sh --go-tag go1.26.0 --no-verify
```

By default, `build-go-detsched.sh` does:

- apply check
- patch apply
- `make.bash`
- `misc/detscheddemo/run_all_demos.sh` (seed + stress + synctest)
- install to `--prefix` (unless `--no-install`)

The patch file is the canonical apply artifact.

## Notes

- This is a patch-distribution repo, not a full Go source mirror.
- External nondeterministic inputs (signals, external I/O, etc.) are still out of scope for determinism guarantees.
