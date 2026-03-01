# Detsched Demos

These demos are intentionally outside the Go source tree so they can live in
this repo (or a different repo) independently from the patch payload.

## Run demos with a patched toolchain

Set a patched Go binary path:

```bash
GO_BIN="$HOME/.local/go-detsched-1.26.0/bin/go"
```

Run reproducibility demo:

```bash
GODEBUG=detsched=1,detschedseed=12345 "$GO_BIN" run ./tests/cmd/seedhash/main.go
```

Run fuzzer-style interleaving demo:

```bash
GODEBUG=detsched=1,detschedfuzz=1,detschedseed=7 "$GO_BIN" run ./tests/cmd/fuzzprobe/main.go
```
