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

Scenario runs are wrapped in `testing/synctest`, so simulated time jumps forward
as soon as all goroutines are blocked on timers/channels. This keeps the demo
fast while still exercising timer-heavy election logic.

## Using Binary Releases / CI Outputs

Do not compile the compiler from source for this demo. Use one of:

1. GitHub release toolchain archives from
   `.github/workflows/release.yml` assets.
2. CI-produced patched `go` binary paths from build artifacts in your workflow
   environment.

Then run the demo with that binary via `GO_BIN` (or equivalent explicit path).

## CI End-to-End Determinism Checks

CI uses `scripts/run-raft-demo-ci.sh` with the patched toolchain to:

1. run each scenario twice with the same seed,
2. assert the expected bug issue code is observed, and
3. assert the summary output is byte-identical across reruns.

Example local invocation against a patched binary:

```bash
./scripts/run-raft-demo-ci.sh --go "$GO_BIN" --seed 7 --log-dir ./ci-logs
```

The log directory contains per-scenario run logs and summary diffs for debugging.
