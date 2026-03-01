#!/usr/bin/env bash
set -euo pipefail

GO_BIN="go"
SEED="${SEED:-7}"
LOG_DIR=""
TIMEOUT_SECS="${TIMEOUT_SECS:-120}"
SEED_START="${SEED_START:-1}"
SEED_COUNT="${SEED_COUNT:-3}"

usage() {
  cat <<'EOF'
Usage: run-raft-demo-ci.sh [options]

Runs deterministic Raft demo checks using a selected Go binary.
This script is intended for CI validation with a patched toolchain.

Options:
  --go <path>        Go binary to use (default: go from PATH)
  --seed <n>         Back-compat alias for --seed-start (default: 7 if used)
  --seed-start <n>   First seed in deterministic sweep (default: 1)
  --seed-count <n>   Number of seeds to test per scenario (default: 3)
  --log-dir <path>   Directory for detailed logs (required)
  --timeout <sec>    Total go test timeout in seconds (default: 120)
  -h, --help         Show help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --go) GO_BIN="$2"; shift 2 ;;
    --seed) SEED="$2"; SEED_START="$2"; shift 2 ;;
    --seed-start) SEED_START="$2"; shift 2 ;;
    --seed-count) SEED_COUNT="$2"; shift 2 ;;
    --log-dir) LOG_DIR="$2"; shift 2 ;;
    --timeout) TIMEOUT_SECS="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage; exit 1 ;;
  esac
done

if [[ -z "$LOG_DIR" ]]; then
  echo "error: --log-dir is required" >&2
  exit 1
fi
if ! [[ "$TIMEOUT_SECS" =~ ^[0-9]+$ ]] || [[ "$TIMEOUT_SECS" -le 0 ]]; then
  echo "error: --timeout must be a positive integer" >&2
  exit 1
fi
if ! [[ "$SEED_START" =~ ^[0-9]+$ ]] || [[ "$SEED_START" -le 0 ]]; then
  echo "error: --seed-start must be a positive integer" >&2
  exit 1
fi
if ! [[ "$SEED_COUNT" =~ ^[0-9]+$ ]] || [[ "$SEED_COUNT" -le 0 ]]; then
  echo "error: --seed-count must be a positive integer" >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
DEMO_DIR="${REPO_ROOT}/demos/raftsim"
mkdir -p "$LOG_DIR"
CACHE_ROOT="${LOG_DIR}/raftsim-go-cache"
mkdir -p "${CACHE_ROOT}/mod" "${CACHE_ROOT}/build"

warm_modules() {
  echo "-- warming raft demo module cache"
  (
    cd "$DEMO_DIR"
    env \
      GOMODCACHE="${CACHE_ROOT}/mod" \
      GOCACHE="${CACHE_ROOT}/build" \
      "$GO_BIN" mod download
  ) > "${LOG_DIR}/raftsim_mod_download.log" 2>&1
}

echo "== Raft demo deterministic CI checks =="
echo "go_bin=${GO_BIN}"
echo "seed_start=${SEED_START}"
echo "seed_count=${SEED_COUNT}"
echo "logs=${LOG_DIR}"
echo "timeout_sec=${TIMEOUT_SECS}"
warm_modules

OUT_FILE="${LOG_DIR}/raftsim_synctest.log"
set +e
(
  cd "$DEMO_DIR"
  timeout "${TIMEOUT_SECS}s" env \
    GODEBUG="" \
    GOMODCACHE="${CACHE_ROOT}/mod" \
    GOCACHE="${CACHE_ROOT}/build" \
    RAFTSIM_SEED_START="${SEED_START}" \
    RAFTSIM_SEED_COUNT="${SEED_COUNT}" \
    RAFTSIM_NODES=5 \
    RAFTSIM_ROUNDS=4 \
    "$GO_BIN" test ./internal/scenarios -count=1 -v -run TestSynctestDeterministicRepro
) > "$OUT_FILE" 2>&1
status=$?
set -e
if [[ "$status" -ne 0 ]]; then
  if [[ "$status" -eq 124 ]]; then
    echo "synctest go test timed out after ${TIMEOUT_SECS}s" >&2
  else
    echo "synctest go test failed with status=${status}" >&2
  fi
  echo "---- recent ${OUT_FILE} ----" >&2
  rg -n "." "$OUT_FILE" -m 60 >&2 || true
  exit "$status"
fi

# Smoke check with detsched enabled to verify patched-runtime execution path.
DETSCHED_OUT="${LOG_DIR}/raftsim_detsched_smoke.log"
set +e
(
  cd "$DEMO_DIR"
  timeout 40s env \
    GODEBUG="detsched=1,detschedseed=7" \
    GOMODCACHE="${CACHE_ROOT}/mod" \
    GOCACHE="${CACHE_ROOT}/build" \
    "$GO_BIN" run ./cmd/raftsim \
      --scenario split_vote \
      --seed 7 \
      --nodes 5 \
      --rounds 2 \
      --synctest=false
) > "$DETSCHED_OUT" 2>&1
status=$?
set -e
if [[ "$status" -ne 0 ]]; then
  echo "detsched smoke run failed with status=${status}" >&2
  echo "---- recent ${DETSCHED_OUT} ----" >&2
  rg -n "." "$DETSCHED_OUT" -m 60 >&2 || true
  exit "$status"
fi
if ! rg -q "issue=RAFT_SPLIT_VOTE_LIVELOCK" "$DETSCHED_OUT"; then
  echo "detsched smoke output missing expected split-vote issue marker" >&2
  rg -n "." "$DETSCHED_OUT" -m 60 >&2 || true
  exit 1
fi

echo "Raft demo deterministic checks passed."
