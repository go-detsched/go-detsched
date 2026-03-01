#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
GO="$ROOT/bin/go"
DEMO="$ROOT/misc/detscheddemo/main.go"

seed="${1:-12345}"
other_seed="${2:-54321}"

run_with_seed() {
  local s="$1"
  GODEBUG="detsched=1,detschedseed=${s}" "$GO" run "$DEMO" -workers=128 -iters=20000 -procs=1
}

echo "Run A with seed=$seed"
h1="$(run_with_seed "$seed")"
echo "hash=$h1"

echo "Run B with same seed=$seed"
h2="$(run_with_seed "$seed")"
echo "hash=$h2"

if [[ "$h1" != "$h2" ]]; then
  echo "FAIL: same seed produced different hashes" >&2
  exit 1
fi

echo "Run C with different seed=$other_seed"
h3="$(run_with_seed "$other_seed")"
echo "hash=$h3"

if [[ "$h1" == "$h3" ]]; then
  echo "WARNING: different seed matched hash (possible but unlikely)"
else
  echo "Different seed produced a different hash as expected."
fi

echo "Deterministic scheduler seed demo passed."
