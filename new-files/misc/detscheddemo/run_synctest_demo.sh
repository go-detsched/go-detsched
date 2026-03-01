#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
GO="$ROOT/bin/go"
TEST_NAME='TestDetSchedSeededBubbleHash'

seed="${1:-12345}"
other_seed="${2:-99999}"
RUN_HASH=""
TEST_BIN=""

ts() {
  date -u +"%Y-%m-%dT%H:%M:%SZ"
}

run_once() {
  local s="$1"
  local label="$2"
  local log_file
  log_file="$(mktemp)"

  echo "[$(ts)] synctest start label=${label} seed=${s}"
  set +e
  GODEBUG="detsched=1,detschedseed=${s}" "$TEST_BIN" -test.run "^${TEST_NAME}$" -test.count=1 -test.v -test.timeout=5m 2>&1 | tee "$log_file"
  local test_status=${PIPESTATUS[0]}
  set -e
  echo "[$(ts)] synctest end label=${label} seed=${s} exit=${test_status}"

  if [[ "$test_status" -ne 0 ]]; then
    echo "[$(ts)] FAIL: go test exited non-zero for label=${label}" >&2
    echo "[$(ts)] Last 120 lines from go test output:" >&2
    tail -n 120 "$log_file" >&2
    rm -f "$log_file"
    return "$test_status"
  fi

  local hash
  hash="$(sed -n 's/.*detsched-hash=\([0-9a-f]\{16\}\).*/\1/p' "$log_file" | tail -n1)"
  if [[ -z "$hash" ]]; then
    echo "[$(ts)] FAIL: could not extract detsched hash for label=${label}" >&2
    echo "[$(ts)] Last 120 lines from go test output:" >&2
    tail -n 120 "$log_file" >&2
    rm -f "$log_file"
    return 1
  fi

  RUN_HASH="$hash"
  echo "[$(ts)] synctest hash label=${label} value=${RUN_HASH}"
  rm -f "$log_file"
}

build_test_binary() {
  TEST_BIN="$(mktemp /tmp/synctest-demo-XXXXXX.test)"
  echo "[$(ts)] building synctest test binary..."
  "$GO" test -c -o "$TEST_BIN" testing/synctest
  chmod +x "$TEST_BIN"
  echo "[$(ts)] built synctest binary at $TEST_BIN"
}

cleanup() {
  if [[ -n "$TEST_BIN" && -f "$TEST_BIN" ]]; then
    rm -f "$TEST_BIN"
  fi
}
trap cleanup EXIT

build_test_binary

echo "Running synctest demo with seed=$seed (run 1)..."
run_once "$seed" "run1"
h1="$RUN_HASH"
echo "hash=$h1"

echo "Running synctest demo with seed=$seed (run 2)..."
run_once "$seed" "run2"
h2="$RUN_HASH"
echo "hash=$h2"

if [[ "$h1" != "$h2" ]]; then
  echo "FAIL: same seed produced different synctest hashes" >&2
  exit 1
fi

echo "Running synctest demo with seed=$other_seed..."
run_once "$other_seed" "run3"
h3="$RUN_HASH"
echo "hash=$h3"

if [[ "$h1" == "$h3" ]]; then
  echo "WARNING: different seed matched hash (possible but unlikely)"
else
  echo "Different seed produced a different synctest hash as expected."
fi

echo "Synctest + deterministic scheduler demo passed."
