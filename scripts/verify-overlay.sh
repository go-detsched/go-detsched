#!/usr/bin/env bash
set -euo pipefail

GO_TAG="go1.26.0"
WORKDIR=""
PATCH_FILE=""
OVERLAY_DIR=""

usage() {
  cat <<'EOF'
Usage: verify-overlay.sh [options]

Options:
  --go-tag <tag>       Upstream Go tag (default: go1.26.0)
  --workdir <path>     Workspace for clone/apply (default: mktemp dir)
  --patch <path>       Patch file (default: repo-root/detsched-only-feature.git.patch)
  --overlay <path>     Overlay dir (default: repo-root/overlay)
  -h, --help           Show help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --go-tag) GO_TAG="$2"; shift 2 ;;
    --workdir) WORKDIR="$2"; shift 2 ;;
    --patch) PATCH_FILE="$2"; shift 2 ;;
    --overlay) OVERLAY_DIR="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage; exit 1 ;;
  esac
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
PATCH_FILE="${PATCH_FILE:-${REPO_ROOT}/detsched-only-feature.git.patch}"
OVERLAY_DIR="${OVERLAY_DIR:-${REPO_ROOT}/overlay}"

if [[ ! -f "$PATCH_FILE" ]]; then
  echo "patch file not found: $PATCH_FILE" >&2
  exit 1
fi
if [[ ! -d "$OVERLAY_DIR" ]]; then
  echo "overlay directory not found: $OVERLAY_DIR" >&2
  exit 1
fi
if [[ ! -f "${OVERLAY_DIR}/FILES.txt" ]]; then
  echo "overlay file list missing: ${OVERLAY_DIR}/FILES.txt" >&2
  exit 1
fi

if [[ -z "${WORKDIR}" ]]; then
  WORKDIR="$(mktemp -d)"
  CLEAN_WORKDIR=1
else
  mkdir -p "$WORKDIR"
  CLEAN_WORKDIR=0
fi

SRC_DIR="${WORKDIR}/go-overlay-verify"
rm -rf "$SRC_DIR"
git clone --depth 1 --branch "$GO_TAG" https://go.googlesource.com/go "$SRC_DIR" >/dev/null 2>&1

(
  cd "$SRC_DIR"
  git apply --check "$PATCH_FILE"
  git apply "$PATCH_FILE"
)

TMP_LIST="${WORKDIR}/patch-files.txt"
awk '/^diff --git /{print $3}' "$PATCH_FILE" | sed 's#^a/##' > "$TMP_LIST"
if ! diff -u "$TMP_LIST" "${OVERLAY_DIR}/FILES.txt" >/dev/null; then
  echo "overlay/FILES.txt does not match patch file list" >&2
  diff -u "$TMP_LIST" "${OVERLAY_DIR}/FILES.txt" || true
  exit 1
fi

while IFS= read -r path; do
  [[ -z "$path" ]] && continue
  if [[ ! -f "${OVERLAY_DIR}/${path}" ]]; then
    echo "missing overlay file: ${OVERLAY_DIR}/${path}" >&2
    exit 1
  fi
  if ! diff -u "${SRC_DIR}/${path}" "${OVERLAY_DIR}/${path}" >/dev/null; then
    echo "overlay mismatch: ${path}" >&2
    diff -u "${SRC_DIR}/${path}" "${OVERLAY_DIR}/${path}" || true
    exit 1
  fi
done < "${OVERLAY_DIR}/FILES.txt"

echo "Overlay verification succeeded for ${GO_TAG}"

if [[ "$CLEAN_WORKDIR" -eq 1 ]]; then
  rm -rf "$WORKDIR"
fi
