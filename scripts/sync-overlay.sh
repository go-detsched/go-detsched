#!/usr/bin/env bash
set -euo pipefail

GO_TAG="go1.26.0"
WORKDIR=""
PATCH_FILE=""
OUTPUT_DIR=""

usage() {
  cat <<'EOF'
Usage: sync-overlay.sh [options]

Options:
  --go-tag <tag>       Upstream Go tag (default: go1.26.0)
  --workdir <path>     Workspace for clone/apply (default: mktemp dir)
  --patch <path>       Patch file (default: repo-root/detsched-only-feature.git.patch)
  --output <path>      Overlay output dir (default: repo-root/overlay)
  -h, --help           Show help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --go-tag) GO_TAG="$2"; shift 2 ;;
    --workdir) WORKDIR="$2"; shift 2 ;;
    --patch) PATCH_FILE="$2"; shift 2 ;;
    --output) OUTPUT_DIR="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage; exit 1 ;;
  esac
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
PATCH_FILE="${PATCH_FILE:-${REPO_ROOT}/detsched-only-feature.git.patch}"
OUTPUT_DIR="${OUTPUT_DIR:-${REPO_ROOT}/overlay}"

if [[ ! -f "$PATCH_FILE" ]]; then
  echo "patch file not found: $PATCH_FILE" >&2
  exit 1
fi

if [[ -z "${WORKDIR}" ]]; then
  WORKDIR="$(mktemp -d)"
  CLEAN_WORKDIR=1
else
  mkdir -p "$WORKDIR"
  CLEAN_WORKDIR=0
fi

SRC_DIR="${WORKDIR}/go-overlay-src"
rm -rf "$SRC_DIR"
git clone --depth 1 --branch "$GO_TAG" https://go.googlesource.com/go "$SRC_DIR" >/dev/null 2>&1

(
  cd "$SRC_DIR"
  git apply --check "$PATCH_FILE"
  git apply "$PATCH_FILE"
)

mkdir -p "$OUTPUT_DIR"
rm -rf "${OUTPUT_DIR:?}/"*

FILES_LIST="${OUTPUT_DIR}/FILES.txt"
awk '/^diff --git /{print $3}' "$PATCH_FILE" | sed 's#^a/##' > "$FILES_LIST"

while IFS= read -r path; do
  [[ -z "$path" ]] && continue
  mkdir -p "${OUTPUT_DIR}/$(dirname "$path")"
  cp -a "${SRC_DIR}/${path}" "${OUTPUT_DIR}/${path}"
done < "$FILES_LIST"

echo "Overlay synced at: $OUTPUT_DIR"

if [[ "$CLEAN_WORKDIR" -eq 1 ]]; then
  rm -rf "$WORKDIR"
fi
