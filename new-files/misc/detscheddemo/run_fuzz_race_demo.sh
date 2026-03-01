#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
GO="$ROOT/bin/go"
DEMO_SRC="$ROOT/misc/detscheddemo/fuzz_race_demo.go"

seed_start="${1:-1}"
seed_end="${2:-200}"
rounds="${3:-500}"
attempts="${4:-6}"
noise="${5:-12}"
fail_threshold="${6:-3}"
scan_mode="${7:-first-fail}" # first-fail | full-scan

DEMO_BIN=""

cleanup() {
  if [[ -n "$DEMO_BIN" && -f "$DEMO_BIN" ]]; then
    rm -f "$DEMO_BIN"
  fi
}
trap cleanup EXIT

build_demo_binary() {
  DEMO_BIN="$(mktemp /tmp/fuzz-race-demo-XXXXXX.bin)"
  "$GO" build -o "$DEMO_BIN" "$DEMO_SRC"
  chmod +x "$DEMO_BIN"
}

run_once() {
  local s="$1"
  GODEBUG="detsched=1,detschedfuzz=1,detschedseed=${s}" \
    "$DEMO_BIN" \
      -rounds="$rounds" \
      -attempts="$attempts" \
      -noise="$noise" \
      -fail-threshold="$fail_threshold"
}

extract_hash() {
  sed -n 's/.*hash=\([0-9a-f]\{16\}\).*/\1/p'
}

build_demo_binary

pass_seed=""
pass_hash=""
fail_seed=""
fail_hash=""
fail_count=0
total_count=0

echo "Scanning seeds ${seed_start}..${seed_end} mode=${scan_mode} rounds=${rounds} attempts=${attempts} noise=${noise} fail_threshold=${fail_threshold}..."
for ((s=seed_start; s<=seed_end; s++)); do
  total_count=$((total_count + 1))
  if out="$(run_once "$s" 2>&1)"; then
    if [[ -z "$pass_seed" ]]; then
      pass_seed="$s"
      pass_hash="$(printf '%s\n' "$out" | extract_hash)"
    fi
    continue
  fi

  fail_count=$((fail_count + 1))
  if [[ -z "$fail_seed" ]]; then
    fail_seed="$s"
    fail_hash="$(printf '%s\n' "$out" | extract_hash)"
    echo "$out"
  fi

  if [[ "$scan_mode" == "first-fail" ]]; then
    break
  fi
done

if [[ "$scan_mode" == "full-scan" ]]; then
  catch_rate_pct="$(awk -v f="$fail_count" -v t="$total_count" 'BEGIN { printf "%.2f", (100.0 * f) / t }')"
  echo "full_scan_summary total_seeds=${total_count} failing_seeds=${fail_count} catch_rate_pct=${catch_rate_pct}"
fi

if [[ -z "$fail_seed" ]]; then
  if [[ "$scan_mode" == "first-fail" ]]; then
    echo "FAIL: no failing seed found in scan window" >&2
    exit 1
  fi
  echo "No failing seed found in full-scan window."
  exit 0
fi

echo "Found failing seed=${fail_seed} hash=${fail_hash}"
echo "Re-running failing seed twice to verify deterministic replay..."

o1="$(run_once "$fail_seed" 2>&1 || true)"
h1="$(printf '%s\n' "$o1" | extract_hash)"
o2="$(run_once "$fail_seed" 2>&1 || true)"
h2="$(printf '%s\n' "$o2" | extract_hash)"

echo "$o1"
echo "$o2"

if [[ "$h1" != "$h2" ]]; then
  echo "FAIL: failing seed hash changed across reruns" >&2
  exit 1
fi

if [[ -n "$pass_seed" ]]; then
  echo "Found passing seed=${pass_seed} hash=${pass_hash}"
fi

echo "Fuzzer race demo passed: reproducible failure at seed=${fail_seed} hash=${h1}."
