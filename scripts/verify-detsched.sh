#!/usr/bin/env bash
set -euo pipefail

GO_TAG="go1.26.0"
WORKDIR=""
PATCH_FILE=""

usage() {
  cat <<'EOF'
Usage: verify-detsched.sh [options]

Options:
  --go-tag <tag>       Upstream Go tag to test (default: go1.26.0)
  --workdir <path>     Workspace for clone/build (default: mktemp dir)
  --patch <path>       Patch file (default: repo-root/detsched-only-feature.git.patch)
  -h, --help           Show help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --go-tag) GO_TAG="$2"; shift 2 ;;
    --workdir) WORKDIR="$2"; shift 2 ;;
    --patch) PATCH_FILE="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage; exit 1 ;;
  esac
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
PATCH_FILE="${PATCH_FILE:-${REPO_ROOT}/detsched-only-feature.git.patch}"

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

echo "Using workdir: $WORKDIR"
echo "Testing tag: $GO_TAG"
echo "Patch: $PATCH_FILE"

SRC_DIR="${WORKDIR}/go-verify"
rm -rf "$SRC_DIR"
git clone --depth 1 --branch "$GO_TAG" https://go.googlesource.com/go "$SRC_DIR"

(
  cd "$SRC_DIR"
  git apply --check "$PATCH_FILE"
  git apply "$PATCH_FILE"
  (cd src && ./make.bash)
  (cd misc/detscheddemo && ./run_all_demos.sh)
)

echo "Verification succeeded for $GO_TAG"

if [[ "$CLEAN_WORKDIR" -eq 1 ]]; then
  rm -rf "$WORKDIR"
fi
