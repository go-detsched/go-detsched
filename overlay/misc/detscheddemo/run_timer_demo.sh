#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
GO="$ROOT/bin/go"
DEMO="$ROOT/misc/detscheddemo/timer_demo.go"

seed="${1:-1}"
other_seed="${2:-2}"

run_once() {
  local s="$1"
  GODEBUG="detsched=1,detschedseed=${s}" "$GO" run "$DEMO" -workers=64 -rounds=2000 -procs=1
}

echo "Running timer demo with seed=$seed (run 1)..."
h1="$(run_once "$seed")"
echo "hash=$h1"

echo "Running timer demo with seed=$seed (run 2)..."
h2="$(run_once "$seed")"
echo "hash=$h2"

if [[ "$h1" != "$h2" ]]; then
  echo "FAIL: same seed produced different timer demo hashes" >&2
  exit 1
fi

echo "Running timer demo with seed=$other_seed..."
h3="$(run_once "$other_seed")"
echo "hash=$h3"

if [[ "$h1" == "$h3" ]]; then
  echo "WARNING: different seed matched hash (possible but unlikely)"
else
  echo "Different seed produced a different timer demo hash as expected."
fi

echo "Timer determinism demo passed."
