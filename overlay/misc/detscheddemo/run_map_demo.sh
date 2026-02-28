#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
GO="$ROOT/bin/go"
DEMO="$ROOT/misc/detscheddemo/map_demo.go"

seed="${1:-1}"
other_seed="${2:-2}"

run_once() {
  local s="$1"
  GODEBUG="detsched=1,detschedseed=${s}" "$GO" run "$DEMO" -entries=50000 -repeats=5 -procs=1
}

echo "Running map demo with seed=$seed (run 1)..."
h1="$(run_once "$seed")"
echo "hash=$h1"

echo "Running map demo with seed=$seed (run 2)..."
h2="$(run_once "$seed")"
echo "hash=$h2"

if [[ "$h1" != "$h2" ]]; then
  echo "FAIL: same seed produced different map iteration hashes" >&2
  exit 1
fi

echo "Running map demo with seed=$other_seed..."
h3="$(run_once "$other_seed")"
echo "hash=$h3"

if [[ "$h1" == "$h3" ]]; then
  echo "WARNING: different seed matched hash (possible but unlikely)"
else
  echo "Different seed produced a different map iteration hash as expected."
fi

echo "Map iteration determinism demo passed."
