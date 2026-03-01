# Raft Deterministic Simulation Demo

This demo shows how the deterministic scheduler patch makes timing-sensitive
distributed bugs reproducible.

It intentionally implements a **minimal, intentionally buggy** Raft-like
cluster:

- leader election
- heartbeat / AppendEntries skeleton
- deterministic failure scenarios

The network is an in-memory fake transport built on
`google.golang.org/grpc/test/bufconn`. Nodes register endpoints and communicate
over `net.Listener`/`net.Conn`-style APIs, so the Raft logic stays close to
real networking semantics.

## Quick Start

Set the patched Go binary you want to use:

```bash
GO_BIN="$HOME/.local/go-detsched-1.26.0/bin/go"
```

Run all scenarios:

```bash
cd demos/raftsim
GODEBUG=detsched=1,detschedseed=7 "$GO_BIN" run ./cmd/raftsim --scenario all --nodes 5 --rounds 4
```

Verbose mode:

```bash
GODEBUG=detsched=1,detschedseed=7 "$GO_BIN" run ./cmd/raftsim --scenario stale_leader --verbose
```

## Scenarios

- `split_vote`: fixed election timeout can livelock in a split vote.
- `stale_leader`: follower incorrectly accepts stale leader append.
- `reorder_commit`: commit index advances without majority replication.

Each scenario prints a deterministic event hash for easy reproduction:

```text
scenario=stale_leader seed=7 status=PASS bug_observed=true issue=RAFT_STALE_LEADER_ACCEPTED hash=... reason="..." evidence="..."
```

## Why `synctest`

By default, CLI runs can be wrapped in `testing/synctest` (`--synctest=true`),
so simulated time jumps forward as soon as all goroutines are blocked on
timers/channels.

For deterministic CI validation, this repo now uses a **proper `go test`**
synctest suite in `internal/scenarios/scenarios_test.go`.

## Using Binary Releases / CI Outputs

Do not compile the compiler from source for this demo. Use one of:

1. GitHub release toolchain archives from
   `.github/workflows/release.yml` assets.
2. CI-produced patched `go` binary paths from build artifacts in your workflow
   environment.

Then run the demo with that binary via `GO_BIN` (or equivalent explicit path).

## CI End-to-End Determinism Checks

CI uses `scripts/run-raft-demo-ci.sh` with the patched toolchain to:

1. run `go test ./internal/scenarios -run TestSynctestDeterministicRepro` once,
2. sweep many seeds per scenario inside one compiled test binary,
3. assert deterministic same-seed replay inside the test, and
4. assert expected bug issue classes are observed in output logs.

You can tune the seed sweep via environment variables consumed by the test:

- `RAFTSIM_SEED_START` (default `1`)
- `RAFTSIM_SEED_COUNT` (default `25`)
- `RAFTSIM_NODES` (default `5`)
- `RAFTSIM_ROUNDS` (default `4`)

Example local invocation against a patched binary:

```bash
./scripts/run-raft-demo-ci.sh --go "$GO_BIN" --seed 7 --log-dir ./ci-logs
```

The log directory contains per-scenario run logs and summary diffs for debugging.
