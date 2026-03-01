#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
GO="$ROOT/bin/go"
DEMO="$ROOT/misc/detscheddemo/stress_demo.go"

seed="${1:-12345}"
other_seed="${2:-54321}"
workers="${3:-256}"
iters="${4:-3000}"

run_once() {
  local s="$1"
  GODEBUG="detsched=1,detschedseed=${s}" "$GO" run "$DEMO" -workers="$workers" -iters="$iters" -procs=1
}

extract_hash() {
  sed -n 's/.*hash=\([0-9a-f]\{16\}\).*/\1/p'
}

extract_switches() {
  sed -n 's/.*switches=\([0-9]\+\).*/\1/p'
}

echo "Running stress demo with seed=$seed (run 1)..."
o1="$(run_once "$seed")"
h1="$(printf '%s\n' "$o1" | extract_hash)"
s1="$(printf '%s\n' "$o1" | extract_switches)"
echo "$o1"

echo "Running stress demo with seed=$seed (run 2)..."
o2="$(run_once "$seed")"
h2="$(printf '%s\n' "$o2" | extract_hash)"
s2="$(printf '%s\n' "$o2" | extract_switches)"
echo "$o2"

if [[ "$h1" != "$h2" ]]; then
  echo "FAIL: same seed produced different stress hashes" >&2
  exit 1
fi

if [[ "$s1" -lt 200000 || "$s2" -lt 200000 ]]; then
  echo "FAIL: stress run did not hit enough context switches" >&2
  exit 1
fi

echo "Running stress demo with seed=$other_seed..."
o3="$(run_once "$other_seed")"
h3="$(printf '%s\n' "$o3" | extract_hash)"
echo "$o3"

if [[ "$h1" == "$h3" ]]; then
  echo "WARNING: different seed matched stress hash (possible but unlikely)"
else
  echo "Different seed produced a different stress hash as expected."
fi

echo "Combined stress determinism demo passed."
