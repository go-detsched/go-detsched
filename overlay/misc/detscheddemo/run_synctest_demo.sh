#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
GO="$ROOT/bin/go"
TEST_NAME='TestDetSchedSeededBubbleHash'

seed="${1:-12345}"
other_seed="${2:-99999}"

run_once() {
  local s="$1"
  GODEBUG="detsched=1,detschedseed=${s}" "$GO" test testing/synctest -run "^${TEST_NAME}$" -count=1 -v \
    | sed -n 's/.*detsched-hash=\([0-9a-f]\{16\}\).*/\1/p'
}

echo "Running synctest demo with seed=$seed (run 1)..."
h1="$(run_once "$seed")"
echo "hash=$h1"

echo "Running synctest demo with seed=$seed (run 2)..."
h2="$(run_once "$seed")"
echo "hash=$h2"

if [[ "$h1" != "$h2" ]]; then
  echo "FAIL: same seed produced different synctest hashes" >&2
  exit 1
fi

echo "Running synctest demo with seed=$other_seed..."
h3="$(run_once "$other_seed")"
echo "hash=$h3"

if [[ "$h1" == "$h3" ]]; then
  echo "WARNING: different seed matched hash (possible but unlikely)"
else
  echo "Different seed produced a different synctest hash as expected."
fi

echo "Synctest + deterministic scheduler demo passed."
