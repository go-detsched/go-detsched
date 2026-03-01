#!/usr/bin/env bash
set -euo pipefail

GO_BIN="go"
SEED_A="${SEED_A:-12345}"
SEED_B="${SEED_B:-99999}"

usage() {
  cat <<'EOF'
Usage: run-tests.sh [options]

Runs concise patch verification tests using a selected Go binary.

Options:
  --go <path>    Go binary to use (default: go from PATH)
  -h, --help     Show help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --go) GO_BIN="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage; exit 1 ;;
  esac
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
SEEDHASH_BIN=""

cleanup() {
  if [[ -n "$SEEDHASH_BIN" && -f "$SEEDHASH_BIN" ]]; then
    rm -f "$SEEDHASH_BIN"
  fi
}
trap cleanup EXIT

build_seedhash_binary() {
  SEEDHASH_BIN="$(mktemp /tmp/detsched-seedhash-XXXXXX.bin)"
  "$GO_BIN" build -o "$SEEDHASH_BIN" "${REPO_ROOT}/tests/cmd/seedhash/main.go"
  chmod +x "$SEEDHASH_BIN"
}

run_seedhash() {
  local seed="$1"
  GODEBUG="detsched=1,detschedseed=${seed}" \
    "$SEEDHASH_BIN" -workers=64 -iters=2000 -procs=1
}

echo "[1/4] seed reproducibility"
build_seedhash_binary
h1="$(run_seedhash "$SEED_A")"
h2="$(run_seedhash "$SEED_A")"
if [[ "$h1" != "$h2" ]]; then
  echo "FAIL: same seed produced different hashes (${h1} vs ${h2})" >&2
  exit 1
fi
echo "ok: same-seed hash=${h1}"

echo "[2/4] seed differentiation"
h3="$(run_seedhash "$SEED_B")"
if [[ "$h1" == "$h3" ]]; then
  echo "FAIL: distinct seeds produced identical hash (${h1})" >&2
  exit 1
fi
echo "ok: distinct-seed hash=${h3}"

echo "[3/4] runtime guardrails"
guards_out="$(
  GOMAXPROCS=8 GODEBUG="detsched=1,detschedseed=${SEED_A}" \
    "$GO_BIN" run "${REPO_ROOT}/tests/cmd/runtimeguards/main.go"
)"
echo "$guards_out"
if [[ "$guards_out" != *"gomaxprocs=1 trace_guard=ok"* ]]; then
  echo "FAIL: runtime guard output mismatch" >&2
  exit 1
fi

echo "[4/4] scheduler fuzz exploration"
pass_count=0
fail_count=0
for ((seed=1; seed<=40; seed++)); do
  set +e
  GODEBUG="detsched=1,detschedfuzz=1,detschedseed=${seed}" \
    "$GO_BIN" run "${REPO_ROOT}/tests/cmd/fuzzprobe/main.go" -rounds=300 -attempts=5 -noise=16 -fail-threshold=2 >/tmp/detsched-fuzzprobe.log 2>&1
  status=$?
  set -e
  if [[ "$status" -eq 0 ]]; then
    pass_count=$((pass_count + 1))
  else
    fail_count=$((fail_count + 1))
  fi
done
echo "fuzz_scan_summary seeds=40 pass=${pass_count} fail=${fail_count}"
if [[ "$pass_count" -eq 0 || "$fail_count" -eq 0 ]]; then
  echo "FAIL: expected both passing and failing seeds in fuzz mode" >&2
  exit 1
fi

echo "All patch verification tests passed."
