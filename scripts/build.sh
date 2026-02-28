#!/usr/bin/env bash
set -euo pipefail

GO_TAG="go1.26.0"
PREFIX="${HOME}/.local/go-detsched-1.26.0"
WORKDIR=""
PATCH_FILE=""
VERIFY=1
INSTALL=1

usage() {
  cat <<'EOF'
Usage: build.sh [options]

Options:
  --go-tag <tag>       Upstream Go tag (default: go1.26.0)
  --prefix <path>      Install prefix for patched GOROOT
  --workdir <path>     Build workspace (default: mktemp dir)
  --patch <path>       Patch file (default: repo-root/detsched.git.patch)
  --no-verify          Skip demo verification run
  --no-install         Build+verify but do not install to prefix
  -h, --help           Show help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --go-tag) GO_TAG="$2"; shift 2 ;;
    --prefix) PREFIX="$2"; shift 2 ;;
    --workdir) WORKDIR="$2"; shift 2 ;;
    --patch) PATCH_FILE="$2"; shift 2 ;;
    --no-verify) VERIFY=0; shift ;;
    --no-install) INSTALL=0; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage; exit 1 ;;
  esac
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
PATCH_FILE="${PATCH_FILE:-${REPO_ROOT}/detsched.git.patch}"

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
echo "Verify demos: $VERIFY"
echo "Install: $INSTALL"
if [[ "$INSTALL" -eq 1 ]]; then
  echo "Install prefix: $PREFIX"
fi

SRC_DIR="${WORKDIR}/go-src"
rm -rf "$SRC_DIR"
git clone --depth 1 --branch "$GO_TAG" https://go.googlesource.com/go "$SRC_DIR"

(
  cd "$SRC_DIR"
  git apply --check "$PATCH_FILE"
  git apply "$PATCH_FILE"
  (cd src && ./make.bash)
  if [[ "$VERIFY" -eq 1 ]]; then
    (cd misc/detscheddemo && ./run_all_demos.sh)
  fi
)

if [[ "$INSTALL" -eq 1 ]]; then
  rm -rf "$PREFIX"
  mkdir -p "$(dirname "$PREFIX")"
  cp -a "$SRC_DIR" "$PREFIX"
fi

if [[ "$INSTALL" -eq 1 ]]; then
  cat <<EOF
Done.
Patched Go installed at: $PREFIX
Use:
  export GOROOT="$PREFIX"
  export PATH="\$GOROOT/bin:\$PATH"
EOF
else
  echo "Done. Build completed in: $SRC_DIR"
fi

if [[ "$CLEAN_WORKDIR" -eq 1 ]]; then
  rm -rf "$WORKDIR"
fi
