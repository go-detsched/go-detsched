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
- `log_truncation`: follower accepts inconsistent previous-log append.
- `vote_no_log_check`: follower grants vote without log up-to-date validation.
- `vote_term_index_comparator`: vote comparator prioritizes index over term.
- `append_timer_reset`: follower fails to reset election timer on append.
- `higher_term_step_down`: leader ignores higher-term append responses.

Each scenario prints a deterministic event hash for easy reproduction:

```text
scenario=stale_leader seed=7 status=PASS bug_observed=true issue=RAFT_STALE_LEADER_ACCEPTED hash=... reason="..." evidence="..."
```

The CLI supports both vulnerable and fixed expectation modes:

```bash
GODEBUG=detsched=1,detschedseed=7 "$GO_BIN" run ./cmd/raftsim --scenario stale_leader --expect-bug=true
GODEBUG=detsched=1,detschedseed=7 "$GO_BIN" run ./cmd/raftsim --scenario stale_leader --expect-bug=false
```

Each run also reports oracle verdict fields:

- `oracle_passed=true|false`
- `oracle_violations=<count>`
- `oracle_first=<first_violation_code|none>`

The oracle is scenario-independent and checks core Raft safety invariants
(single leader per term, valid/contiguous logs, committed-entry consistency,
and majority-replication for committed indexes).

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

You can also fetch release assets directly with `gh`:

```bash
TAG="$(gh release list --limit 1 --json tagName --jq '.[0].tagName')"
gh release download "$TAG" --pattern "go-detsched-go1.26.0-linux-amd64.tar.gz" --pattern "SHA256SUMS"
sha256sum -c SHA256SUMS --ignore-missing
tar -xzf go-detsched-go1.26.0-linux-amd64.tar.gz
GO_BIN="$PWD/go-detsched-go1.26.0-linux-amd64/bin/go"
```

## Instructional Patch Series

This repo now includes a numbered teaching patch sequence in:

- `demos/raftsim/patch-series/stages.tsv`
- `demos/raftsim/patch-series/0001-*.patch` through `0008-*.patch`

Each stage preserves one explicit fix step:

1. prove bug in vulnerable baseline,
2. apply the matching stage patch,
3. prove fixed behavior.

Run the full staged proof locally:

```bash
./scripts/run-raft-patch-series-ci.sh --go "$GO_BIN" --seed-start 1 --seed-count 100 --nodes 5 --rounds 6 --log-dir ./ci-logs/patch-series
```

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

For bug-then-fix instructional checks, CI also runs:

```bash
./scripts/run-raft-patch-series-ci.sh --go "$GO_DETSCHED_WORKDIR/go-src/bin/go" --seed-start 1 --seed-count 100 --nodes 5 --rounds 6 --log-dir ./ci-logs/patch-series
```
