#!/usr/bin/env bash
set -euo pipefail

GO_BIN="${GO_BIN:-}"
RELEASE_TAG="${RELEASE_TAG:-latest}"
LOG_DIR=""
WORK_DIR=""
SEED="${SEED:-7}"
NODES="${NODES:-5}"
ROUNDS="${ROUNDS:-4}"
GO_TAG="${GO_TAG:-go1.26.0}"
PYTHON_BIN="${PYTHON_BIN:-python3}"

usage() {
  cat <<'EOF'
Usage: run-raft-patch-series-ci.sh [options]

Runs the instructional Raft patch series:
1) prove each bug in vulnerable baseline,
2) apply the matching numbered patch,
3) prove the bug is fixed.

Options:
  --go <path>          Use this Go binary (skip release download)
  --release-tag <tag>  Release tag to download (default: latest)
  --seed <n>           Deterministic seed (default: 7)
  --nodes <n>          Raft node count (default: 5)
  --rounds <n>         Proposal rounds (default: 4)
  --work-dir <path>    Existing work directory (default: mktemp)
  --log-dir <path>     Log output directory (required)
  -h, --help           Show help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --go) GO_BIN="$2"; shift 2 ;;
    --release-tag) RELEASE_TAG="$2"; shift 2 ;;
    --seed) SEED="$2"; shift 2 ;;
    --nodes) NODES="$2"; shift 2 ;;
    --rounds) ROUNDS="$2"; shift 2 ;;
    --work-dir) WORK_DIR="$2"; shift 2 ;;
    --log-dir) LOG_DIR="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage; exit 1 ;;
  esac
done

if [[ -z "$LOG_DIR" ]]; then
  echo "error: --log-dir is required" >&2
  exit 1
fi
if ! [[ "$SEED" =~ ^[0-9]+$ ]] || [[ "$SEED" -le 0 ]]; then
  echo "error: --seed must be a positive integer" >&2
  exit 1
fi
if ! [[ "$NODES" =~ ^[0-9]+$ ]] || [[ "$NODES" -lt 3 ]]; then
  echo "error: --nodes must be an integer >= 3" >&2
  exit 1
fi
if ! [[ "$ROUNDS" =~ ^[0-9]+$ ]] || [[ "$ROUNDS" -le 0 ]]; then
  echo "error: --rounds must be a positive integer" >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
PATCH_SERIES_DIR="${REPO_ROOT}/demos/raftsim/patch-series"
STAGES_FILE="${PATCH_SERIES_DIR}/stages.tsv"
mkdir -p "$LOG_DIR"

if [[ ! -f "$STAGES_FILE" ]]; then
  echo "error: missing stage manifest at $STAGES_FILE" >&2
  exit 1
fi

resolve_release_tag() {
  if [[ "$RELEASE_TAG" != "latest" ]]; then
    echo "$RELEASE_TAG"
    return 0
  fi
  gh release list --limit 1 --json tagName --jq '.[0].tagName'
}

download_go_bin() {
  local release_dir release_name repo_name go_asset resolved_tag
  if ! command -v gh >/dev/null 2>&1; then
    echo "error: gh CLI is required to download release binaries" >&2
    exit 1
  fi
  release_dir="${LOG_DIR}/release-assets"
  mkdir -p "$release_dir"
  repo_name="$(gh repo view --json nameWithOwner --jq '.nameWithOwner')"
  resolved_tag="$(resolve_release_tag)"
  case "$(uname -s)-$(uname -m)" in
    Linux-x86_64) go_asset="go-detsched-${GO_TAG}-linux-amd64.tar.gz" ;;
    Darwin-arm64) go_asset="go-detsched-${GO_TAG}-darwin-arm64.tar.gz" ;;
    *)
      echo "error: unsupported platform for release asset auto-download: $(uname -s)-$(uname -m)" >&2
      exit 1
      ;;
  esac

  gh release download "$resolved_tag" \
    --repo "$repo_name" \
    --pattern "$go_asset" \
    --pattern "SHA256SUMS" \
    --dir "$release_dir" \
    --clobber

  (
    cd "$release_dir"
    sha256sum -c SHA256SUMS --ignore-missing
    tar -xzf "$go_asset"
  ) > "${LOG_DIR}/release_download.log" 2>&1

  release_name="${go_asset%.tar.gz}"
  GO_BIN="${release_dir}/${release_name}/bin/go"
  if [[ ! -x "$GO_BIN" ]]; then
    echo "error: downloaded Go binary not found at $GO_BIN" >&2
    exit 1
  fi
}

if [[ -z "$GO_BIN" ]]; then
  download_go_bin
fi
if [[ ! -x "$GO_BIN" ]]; then
  echo "error: --go path is not executable: $GO_BIN" >&2
  exit 1
fi
if ! command -v "$PYTHON_BIN" >/dev/null 2>&1; then
  echo "error: python interpreter not found: $PYTHON_BIN" >&2
  exit 1
fi

if [[ -z "$WORK_DIR" ]]; then
  WORK_DIR="$(mktemp -d /tmp/raft-patch-series-XXXXXX)"
fi
mkdir -p "$WORK_DIR"
WORK_REPO="${WORK_DIR}/repo"
rm -rf "$WORK_REPO"
git -C "$REPO_ROOT" worktree add --detach "$WORK_REPO" HEAD >/dev/null
cleanup_worktree() {
  git -C "$REPO_ROOT" worktree remove --force "$WORK_REPO" >/dev/null 2>&1 || true
}
trap cleanup_worktree EXIT

echo "== Raft instructional patch-series check =="
echo "go_bin=${GO_BIN}"
echo "log_dir=${LOG_DIR}"
echo "work_repo=${WORK_REPO}"
echo "seed=${SEED}"
echo "nodes=${NODES}"
echo "rounds=${ROUNDS}"

summary_file="${LOG_DIR}/patch_series_summary.log"
executed_stages=0
{
  echo "seed=${SEED}"
  echo "nodes=${NODES}"
  echo "rounds=${ROUNDS}"
  echo "stages_file=${STAGES_FILE}"
} > "$summary_file"

run_stage() {
  local step scenario patch bug_issue fixed_issue description
  local bug_log fixed_log patch_path
  step="$1"
  scenario="$2"
  patch="$3"
  bug_issue="$4"
  fixed_issue="$5"
  description="$6"

  echo "-- stage=${step} scenario=${scenario} patch=${patch} description=${description}"
  bug_log="${LOG_DIR}/stage-${step}-${scenario}-bug.log"
  fixed_log="${LOG_DIR}/stage-${step}-${scenario}-fixed.log"
  patch_path="${WORK_REPO}/demos/raftsim/patch-series/${patch}"

  set +e
  (
    cd "${WORK_REPO}/demos/raftsim"
    GODEBUG="detsched=1,detschedseed=${SEED}" \
      "$GO_BIN" run ./cmd/raftsim \
      --scenario "${scenario}" \
      --seed "${SEED}" \
      --nodes "${NODES}" \
      --rounds "${ROUNDS}" \
      --expect-bug=true \
      --synctest=true
  ) > "$bug_log" 2>&1
  local bug_status=$?
  set -e
  if [[ "$bug_status" -ne 0 ]]; then
    echo "error: vulnerable stage failed unexpectedly (stage=${step} scenario=${scenario})" >&2
    rg -n "." "$bug_log" -m 80 >&2 || true
    exit "$bug_status"
  fi
  if ! rg -q "status=PASS" "$bug_log"; then
    echo "error: vulnerable stage did not report PASS (stage=${step} scenario=${scenario})" >&2
    rg -n "." "$bug_log" -m 80 >&2 || true
    exit 1
  fi
  if ! rg -q "issue=${bug_issue}" "$bug_log"; then
    echo "error: vulnerable stage missing expected issue code ${bug_issue}" >&2
    rg -n "." "$bug_log" -m 80 >&2 || true
    exit 1
  fi

  if [[ ! -f "$patch_path" ]]; then
    echo "error: stage patch file missing: $patch_path" >&2
    exit 1
  fi
  apply_stage_patch "$step" "$patch_path"

  set +e
  (
    cd "${WORK_REPO}/demos/raftsim"
    GODEBUG="detsched=1,detschedseed=${SEED}" \
      "$GO_BIN" run ./cmd/raftsim \
      --scenario "${scenario}" \
      --seed "${SEED}" \
      --nodes "${NODES}" \
      --rounds "${ROUNDS}" \
      --expect-bug=false \
      --synctest=true
  ) > "$fixed_log" 2>&1
  local fixed_status=$?
  set -e
  if [[ "$fixed_status" -ne 0 ]]; then
    echo "error: fixed stage failed (stage=${step} scenario=${scenario})" >&2
    rg -n "." "$fixed_log" -m 80 >&2 || true
    exit "$fixed_status"
  fi
  if ! rg -q "status=PASS" "$fixed_log"; then
    echo "error: fixed stage did not report PASS (stage=${step} scenario=${scenario})" >&2
    rg -n "." "$fixed_log" -m 80 >&2 || true
    exit 1
  fi
  if ! rg -q "issue=${fixed_issue}" "$fixed_log"; then
    echo "error: fixed stage missing expected issue code ${fixed_issue}" >&2
    rg -n "." "$fixed_log" -m 80 >&2 || true
    exit 1
  fi
  if rg -q "issue=${bug_issue}" "$fixed_log"; then
    echo "error: fixed stage still reports bug issue code ${bug_issue}" >&2
    rg -n "." "$fixed_log" -m 80 >&2 || true
    exit 1
  fi
}

replace_once() {
  local target old new
  target="$1"
  old="$2"
  new="$3"
  "$PYTHON_BIN" - "$target" "$old" "$new" <<'PY'
from pathlib import Path
import sys

target = Path(sys.argv[1])
old = sys.argv[2]
new = sys.argv[3]
content = target.read_text()
if old not in content:
    raise SystemExit(f"replacement source not found in {target}")
target.write_text(content.replace(old, new, 1))
PY
}

apply_stage_patch() {
  local step patch_path node_file scenarios_file
  step="$1"
  patch_path="$2"
  node_file="${WORK_REPO}/demos/raftsim/internal/raft/node.go"
  scenarios_file="${WORK_REPO}/demos/raftsim/internal/scenarios/scenarios.go"
  echo "applying stage patch logic for step=${step} from ${patch_path}"
  case "$step" in
    1)
      replace_once "$node_file" $'func (n *Node) electionTimeout() time.Duration {\n\tif n.bugs.FixedElectionTimeout {\n\t\treturn n.electionBase\n\t}\n\tif n.electionJitter <= 0 {\n\t\treturn n.electionBase\n\t}\n\treturn n.electionBase + time.Duration(n.rng.Int63n(int64(n.electionJitter)))\n}\n' $'func (n *Node) electionTimeout() time.Duration {\n\tif n.electionJitter <= 0 {\n\t\treturn n.electionBase\n\t}\n\treturn n.electionBase + time.Duration(n.rng.Int63n(int64(n.electionJitter)))\n}\n'
      replace_once "$scenarios_file" $'cluster, cancel, err := startClusterWithMessageFaults(nodeIDs, cfg.Seed, raft.BugConfig{\n\t\tFixedElectionTimeout: true,\n\t}, func(from, to string, msgType raft.MessageType, seq uint64) (bool, time.Duration) {\n\t\tif msgType == raft.MsgRequestVote {\n\t\t\treturn true, 0\n\t\t}\n\t\treturn false, 0\n\t})\n' $'cluster, cancel, err := startClusterWithMessageFaults(nodeIDs, cfg.Seed, raft.BugConfig{\n\t\tFixedElectionTimeout: true,\n\t}, func(from, to string, msgType raft.MessageType, seq uint64) (bool, time.Duration) {\n\t\treturn false, 0\n\t})\n'
      ;;
    2)
      replace_once "$node_file" $'if req.Term < n.term && !n.bugs.AcceptStaleLeader {\n\t\treturn Message{\n\t\t\tType:         MsgAppendResult,\n\t\t\tTerm:         n.term,\n\t\t\tSuccess:      false,\n\t\t\tRejectReason: "stale leader term",\n\t\t\tMatchIndex:   len(n.log),\n\t\t}\n\t}\n\tif req.Term < n.term && n.bugs.AcceptStaleLeader {\n\t\tn.eventf("bug_accept_stale node=%s leader=%s leader_term=%d local_term=%d", n.id, req.LeaderID, req.Term, n.term)\n\t}\n' $'if req.Term < n.term {\n\t\treturn Message{\n\t\t\tType:         MsgAppendResult,\n\t\t\tTerm:         n.term,\n\t\t\tSuccess:      false,\n\t\t\tRejectReason: "stale leader term",\n\t\t\tMatchIndex:   len(n.log),\n\t\t}\n\t}\n'
      ;;
    3)
      replace_once "$node_file" $'if n.bugs.CommitOnSingleAck {\n\t\t// Intentionally buggy commit rule for demo purposes.\n\t\tif acks >= 2 {\n\t\t\tn.commitIndex = entry.Index\n\t\t\tn.eventf("commit_buggy leader=%s index=%d acks=%d majority=%d", n.id, entry.Index, acks, majority)\n\t\t\treturn nil\n\t\t}\n\t\treturn fmt.Errorf("not enough acknowledgements for buggy commit: %d", acks)\n\t}\n\n' $''
      ;;
    4)
      replace_once "$node_file" $'if req.PrevLogIndex > len(n.log) {\n\t\tif !n.bugs.UnsafeLogTruncation {\n\t\t\treturn Message{\n\t\t\t\tType:         MsgAppendResult,\n\t\t\t\tTerm:         n.term,\n\t\t\t\tSuccess:      false,\n\t\t\t\tRejectReason: "prev log index missing",\n\t\t\t\tMatchIndex:   len(n.log),\n\t\t\t}\n\t\t}\n\t\tn.eventf(\n\t\t\t"bug_accept_inconsistent_prev node=%s leader=%s prev_idx=%d local_len=%d",\n\t\t\tn.id,\n\t\t\treq.LeaderID,\n\t\t\treq.PrevLogIndex,\n\t\t\tlen(n.log),\n\t\t)\n\t} else if req.PrevLogIndex > 0 && n.log[req.PrevLogIndex-1].Term != req.PrevLogTerm {\n\t\tif !n.bugs.UnsafeLogTruncation {\n\t\t\treturn Message{\n\t\t\t\tType:         MsgAppendResult,\n\t\t\t\tTerm:         n.term,\n\t\t\t\tSuccess:      false,\n\t\t\t\tRejectReason: "prev log term mismatch",\n\t\t\t\tMatchIndex:   len(n.log),\n\t\t\t}\n\t\t}\n\t\tn.eventf(\n\t\t\t"bug_accept_prev_term_mismatch node=%s leader=%s prev_idx=%d prev_term=%d local_prev_term=%d",\n\t\t\tn.id,\n\t\t\treq.LeaderID,\n\t\t\treq.PrevLogIndex,\n\t\t\treq.PrevLogTerm,\n\t\t\tn.log[req.PrevLogIndex-1].Term,\n\t\t)\n\t}\n' $'if req.PrevLogIndex > len(n.log) {\n\t\treturn Message{\n\t\t\tType:         MsgAppendResult,\n\t\t\tTerm:         n.term,\n\t\t\tSuccess:      false,\n\t\t\tRejectReason: "prev log index missing",\n\t\t\tMatchIndex:   len(n.log),\n\t\t}\n\t} else if req.PrevLogIndex > 0 && n.log[req.PrevLogIndex-1].Term != req.PrevLogTerm {\n\t\treturn Message{\n\t\t\tType:         MsgAppendResult,\n\t\t\tTerm:         n.term,\n\t\t\tSuccess:      false,\n\t\t\tRejectReason: "prev log term mismatch",\n\t\t\tMatchIndex:   len(n.log),\n\t\t}\n\t}\n'
      replace_once "$node_file" $'if e.Index > len(n.log)+1 {\n\t\t\tif !n.bugs.UnsafeLogTruncation {\n\t\t\t\treturn Message{\n\t\t\t\t\tType:         MsgAppendResult,\n\t\t\t\t\tTerm:         n.term,\n\t\t\t\t\tSuccess:      false,\n\t\t\t\t\tRejectReason: "log gap in append entries",\n\t\t\t\t\tMatchIndex:   len(n.log),\n\t\t\t\t}\n\t\t\t}\n\t\t\tn.eventf(\n\t\t\t\t"bug_accept_log_gap node=%s leader=%s entry_idx=%d local_len=%d",\n\t\t\t\tn.id,\n\t\t\t\treq.LeaderID,\n\t\t\t\te.Index,\n\t\t\t\tlen(n.log),\n\t\t\t)\n\t\t}\n' $'if e.Index > len(n.log)+1 {\n\t\t\treturn Message{\n\t\t\t\tType:         MsgAppendResult,\n\t\t\t\tTerm:         n.term,\n\t\t\t\tSuccess:      false,\n\t\t\t\tRejectReason: "log gap in append entries",\n\t\t\t\tMatchIndex:   len(n.log),\n\t\t\t}\n\t\t}\n'
      replace_once "$node_file" $'if existing.Term != e.Term || existing.Data != e.Data {\n\t\t\t\tif n.bugs.UnsafeLogTruncation {\n\t\t\t\t\tn.log[e.Index-1] = e\n\t\t\t\t\tn.eventf(\n\t\t\t\t\t\t"bug_partial_overwrite node=%s leader=%s index=%d old_term=%d new_term=%d",\n\t\t\t\t\t\tn.id,\n\t\t\t\t\t\treq.LeaderID,\n\t\t\t\t\t\te.Index,\n\t\t\t\t\t\texisting.Term,\n\t\t\t\t\t\te.Term,\n\t\t\t\t\t)\n\t\t\t\t\tcontinue\n\t\t\t\t}\n\t\t\t\tn.log = append(n.log[:e.Index-1], req.Entries[i:]...)\n\t\t\t\tbreak\n\t\t\t}\n' $'if existing.Term != e.Term || existing.Data != e.Data {\n\t\t\t\tn.log = append(n.log[:e.Index-1], req.Entries[i:]...)\n\t\t\t\tbreak\n\t\t\t}\n'
      ;;
    *)
      echo "error: unknown patch stage ${step}" >&2
      exit 1
      ;;
  esac
}

while IFS='|' read -r step scenario patch bug_issue fixed_issue description; do
  if [[ "$step" == "step_id" ]]; then
    continue
  fi
  if [[ -z "$step" || -z "$scenario" || -z "$patch" || -z "$bug_issue" || -z "$fixed_issue" || -z "$description" ]]; then
    echo "error: invalid stage row in ${STAGES_FILE}: step=${step} scenario=${scenario} patch=${patch}" >&2
    exit 1
  fi
  if ! [[ "$step" =~ ^[0-9]+$ ]]; then
    echo "error: non-numeric step in ${STAGES_FILE}: ${step}" >&2
    exit 1
  fi
  expected_step=$((executed_stages + 1))
  if [[ "$step" -ne "$expected_step" ]]; then
    echo "error: non-sequential step order in ${STAGES_FILE}: got ${step}, expected ${expected_step}" >&2
    exit 1
  fi
  run_stage "$step" "$scenario" "$patch" "$bug_issue" "$fixed_issue" "$description"
  executed_stages=$((executed_stages + 1))
  echo "stage_${step}=scenario:${scenario},patch:${patch},bug_issue:${bug_issue},fixed_issue:${fixed_issue},result:PASS" >> "$summary_file"
done < "$STAGES_FILE"

if [[ "${executed_stages}" -eq 0 ]]; then
  echo "error: no stages executed from ${STAGES_FILE}" >&2
  exit 1
fi
echo "executed_stages=${executed_stages}" >> "$summary_file"

echo "Raft instructional patch-series checks passed."
