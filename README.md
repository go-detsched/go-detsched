# go-detsched

Deterministic scheduler patch distribution for Go `1.26.0`.

This repository is meant to be the practical way to use and maintain the `detsched` runtime feature without carrying a long-lived full Go fork.

## What this repo contains

- `detsched-only-feature.git.patch`: git-native patch (includes file modes)
- `DETSCHED_FEATURE.md`: implementation and design notes
- `scripts/build-go-detsched.sh`: reproducible build script
- `scripts/verify-detsched.sh`: applies patch, builds, and runs demos
- CI workflow to continuously validate patch applicability

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

## Verify deterministic demos

```bash
./scripts/verify-detsched.sh --go-tag go1.26.0
```

This runs:

- apply check
- patch apply
- `make.bash`
- `misc/detscheddemo/run_all_demos.sh`

## Compatibility policy

- Primary target today: `go1.26.0`
- Each release of this repo should map to exactly one upstream Go tag.
- Keep changelog notes for rebases when upstream internals move.

## Suggested release tags

Use tags like:

- `v1.26.0-detsched.1`
- `v1.26.0-detsched.2`

## Notes

- This is a patch-distribution repo, not a full Go source mirror.
- External nondeterministic inputs (signals, external I/O, etc.) are still out of scope for determinism guarantees.
