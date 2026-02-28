#!/usr/bin/env bash
set -euo pipefail

GO_TAG="go1.26.0"
PREFIX="${HOME}/.local/go-detsched-1.26.0"
WORKDIR=""
PATCH_FILE=""

usage() {
  cat <<'EOF'
Usage: build-go-detsched.sh [options]

Options:
  --go-tag <tag>       Upstream Go tag (default: go1.26.0)
  --prefix <path>      Install prefix for patched GOROOT
  --workdir <path>     Build workspace (default: mktemp dir)
  --patch <path>       Patch file (default: repo-root/detsched-only-feature.git.patch)
  -h, --help           Show help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --go-tag) GO_TAG="$2"; shift 2 ;;
    --prefix) PREFIX="$2"; shift 2 ;;
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
echo "Target tag: $GO_TAG"
echo "Patch: $PATCH_FILE"
echo "Install prefix: $PREFIX"

SRC_DIR="${WORKDIR}/go-src"
rm -rf "$SRC_DIR"
git clone --depth 1 --branch "$GO_TAG" https://go.googlesource.com/go "$SRC_DIR"

(
  cd "$SRC_DIR"
  git apply --check "$PATCH_FILE"
  git apply "$PATCH_FILE"
  (cd src && ./make.bash)
)

rm -rf "$PREFIX"
mkdir -p "$(dirname "$PREFIX")"
cp -a "$SRC_DIR" "$PREFIX"

cat <<EOF
Done.
Patched Go installed at: $PREFIX
Use:
  export GOROOT="$PREFIX"
  export PATH="\$GOROOT/bin:\$PATH"
EOF

if [[ "$CLEAN_WORKDIR" -eq 1 ]]; then
  rm -rf "$WORKDIR"
fi
