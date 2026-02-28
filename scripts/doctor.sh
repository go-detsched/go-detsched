#!/usr/bin/env bash
set -euo pipefail

echo "go binary: $(command -v go || echo 'not found')"
if ! command -v go >/dev/null 2>&1; then
  exit 1
fi

echo "go version: $(go version)"
echo "go env GOROOT: $(go env GOROOT)"

TMP="$(mktemp /tmp/detsched-doctor-XXXX.go)"
cat > "$TMP" <<'EOF'
package main

import "fmt"

func main() {
	fmt.Println("doctor: hello")
}
EOF

echo "smoke run without detsched:"
go run "$TMP"

echo "smoke run with detsched:"
GODEBUG=detsched=1,detschedseed=12345 go run "$TMP"

rm -f "$TMP"
echo "doctor: ok"
