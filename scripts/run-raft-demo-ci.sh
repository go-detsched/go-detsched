#!/usr/bin/env bash
set -euo pipefail

GO_BIN="go"
SEED="${SEED:-7}"
LOG_DIR=""
TIMEOUT_SECS="${TIMEOUT_SECS:-45}"

usage() {
  cat <<'EOF'
Usage: run-raft-demo-ci.sh [options]

Runs deterministic Raft demo checks using a selected Go binary.
This script is intended for CI validation with a patched toolchain.

Options:
  --go <path>        Go binary to use (default: go from PATH)
  --seed <n>         Scenario seed (default: 7)
  --log-dir <path>   Directory for detailed logs (required)
  --timeout <sec>    Per-run timeout in seconds (default: 45)
  -h, --help         Show help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --go) GO_BIN="$2"; shift 2 ;;
    --seed) SEED="$2"; shift 2 ;;
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

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
DEMO_DIR="${REPO_ROOT}/demos/raftsim"
mkdir -p "$LOG_DIR"

run_one() {
  local scenario="$1"
  local run_name="$2"
  local out_file="${LOG_DIR}/raftsim_${scenario}_${run_name}.log"
  local started
  started="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "run_start scenario=${scenario} run=${run_name} timeout_sec=${TIMEOUT_SECS} at=${started}" >&2
  set +e
  (
    cd "$DEMO_DIR"
    timeout "${TIMEOUT_SECS}s" env GODEBUG="detsched=1,detschedseed=${SEED}" \
      "$GO_BIN" run ./cmd/raftsim \
        --scenario "$scenario" \
        --seed "$SEED" \
        --nodes 5 \
        --rounds 4 \
        --verbose
  ) > "$out_file" 2>&1
  local status=$?
  set -e
  if [[ "$status" -ne 0 ]]; then
    if [[ "$status" -eq 124 ]]; then
      echo "scenario=${scenario} run=${run_name} timed out after ${TIMEOUT_SECS}s" >&2
    else
      echo "scenario=${scenario} run=${run_name} failed with status=${status}" >&2
    fi
    echo "---- recent ${out_file} ----" >&2
    rg -n "." "$out_file" -m 30 >&2 || true
    return "$status"
  fi
  local ended
  ended="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "run_end scenario=${scenario} run=${run_name} at=${ended}" >&2
  echo "$out_file"
}

expected_issue_for() {
  case "$1" in
    split_vote) echo "RAFT_SPLIT_VOTE_LIVELOCK" ;;
    stale_leader) echo "RAFT_STALE_LEADER_ACCEPTED" ;;
    reorder_commit) echo "RAFT_COMMIT_WITHOUT_MAJORITY" ;;
    *) echo "UNKNOWN" ;;
  esac
}

assert_summary_line() {
  local scenario="$1"
  local expected_issue
  expected_issue="$(expected_issue_for "$scenario")"
  local file="$2"

  if ! rg -q "^scenario=${scenario} " "$file"; then
    echo "missing summary line for scenario=${scenario} in ${file}" >&2
    return 1
  fi
  if ! rg -q "status=PASS bug_observed=true issue=${expected_issue}" "$file"; then
    echo "scenario=${scenario} did not report expected issue=${expected_issue}" >&2
    rg "^scenario=" "$file" || true
    return 1
  fi
}

compare_replay() {
  local scenario="$1"
  local run1="$2"
  local run2="$3"
  local s1="${LOG_DIR}/raftsim_${scenario}_summary_run1.log"
  local s2="${LOG_DIR}/raftsim_${scenario}_summary_run2.log"

  rg "^scenario=" "$run1" > "$s1"
  rg "^scenario=" "$run2" > "$s2"
  diff -u "$s1" "$s2" > "${LOG_DIR}/raftsim_${scenario}_summary.diff" || {
    echo "scenario=${scenario} produced non-deterministic summary output across reruns" >&2
    return 1
  }
}

echo "== Raft demo deterministic CI checks =="
echo "go_bin=${GO_BIN}"
echo "seed=${SEED}"
echo "logs=${LOG_DIR}"
echo "timeout_sec=${TIMEOUT_SECS}"

scenarios=(split_vote stale_leader reorder_commit)
for scenario in "${scenarios[@]}"; do
  echo "-- scenario=${scenario} run1"
  run1_file="$(run_one "$scenario" "run1")"
  assert_summary_line "$scenario" "$run1_file"

  echo "-- scenario=${scenario} run2"
  run2_file="$(run_one "$scenario" "run2")"
  assert_summary_line "$scenario" "$run2_file"

  compare_replay "$scenario" "$run1_file" "$run2_file"
done

echo "Raft demo deterministic checks passed."
