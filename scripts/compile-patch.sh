#!/usr/bin/env bash
set -euo pipefail

GO_TAG="go1.26.0"
WORKDIR=""
INLINE_PATCH=""
NEW_FILES_DIR=""
OUTPUT_PATCH=""

usage() {
  cat <<'EOF'
Usage: compile-patch.sh [options]

Generate detsched.git.patch from:
  1) inline edits patch (modified upstream files only), and
  2) dedicated new-files directory (brand new upstream files).

Options:
  --go-tag <tag>         Upstream Go tag to compile against (default: go1.26.0)
  --workdir <path>       Workspace directory (default: mktemp dir)
  --inline <path>        Inline patch source (default: repo-root/patches/detsched.inline.patch)
  --new-files <path>     New files source tree (default: repo-root/new-files)
  --output <path>        Output compiled patch (default: repo-root/detsched.git.patch)
  -h, --help             Show help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --go-tag) GO_TAG="$2"; shift 2 ;;
    --workdir) WORKDIR="$2"; shift 2 ;;
    --inline) INLINE_PATCH="$2"; shift 2 ;;
    --new-files) NEW_FILES_DIR="$2"; shift 2 ;;
    --output) OUTPUT_PATCH="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage; exit 1 ;;
  esac
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
INLINE_PATCH="${INLINE_PATCH:-${REPO_ROOT}/patches/detsched.inline.patch}"
NEW_FILES_DIR="${NEW_FILES_DIR:-${REPO_ROOT}/new-files}"
OUTPUT_PATCH="${OUTPUT_PATCH:-${REPO_ROOT}/detsched.git.patch}"

if [[ ! -f "$INLINE_PATCH" ]]; then
  echo "inline patch not found: $INLINE_PATCH" >&2
  exit 1
fi
if [[ ! -d "$NEW_FILES_DIR" ]]; then
  echo "new files directory not found: $NEW_FILES_DIR" >&2
  exit 1
fi

if [[ -z "$WORKDIR" ]]; then
  WORKDIR="$(mktemp -d)"
  CLEAN_WORKDIR=1
else
  mkdir -p "$WORKDIR"
  CLEAN_WORKDIR=0
fi

SRC_DIR="${WORKDIR}/go-src"
rm -rf "$SRC_DIR"
git clone --depth 1 --branch "$GO_TAG" https://go.googlesource.com/go "$SRC_DIR" >/dev/null

(
  cd "$SRC_DIR"

  git apply --check "$INLINE_PATCH"
  git apply "$INLINE_PATCH"

  cp -a "${NEW_FILES_DIR}/." "$SRC_DIR/"

  git add -A
  git diff --cached --full-index --binary > "$OUTPUT_PATCH"
)

echo "Compiled patch written to: $OUTPUT_PATCH"

if [[ "$CLEAN_WORKDIR" -eq 1 ]]; then
  rm -rf "$WORKDIR"
fi
