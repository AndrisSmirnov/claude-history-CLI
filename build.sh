#!/usr/bin/env bash
# Builds hist with the version baked in.
#   ./build.sh            -> bin/hist (this machine)
#   ./build.sh install    -> also copies it to ~/go/bin (make sure it is in PATH)
#   ./build.sh dist       -> dist/hist-<os>-<arch> for macOS and Linux (arm64 + amd64)
set -euo pipefail
cd "$(dirname "$0")"

version="$(git describe --tags --always --dirty 2>/dev/null || date +%Y%m%d-%H%M)"
ldflags="-s -w -X main.version=${version}"
go vet ./...
go test ./...

if [[ "${1:-}" == "dist" ]]; then
  rm -rf dist && mkdir -p dist
  for target in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64; do
    os="${target%/*}" arch="${target#*/}"
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -ldflags "$ldflags" -o "dist/hist-${os}-${arch}" .
    echo "built dist/hist-${os}-${arch} (${version})"
  done
  exit 0
fi

go build -ldflags "$ldflags" -o bin/hist .
echo "built bin/hist (${version})"

if [[ "${1:-}" == "install" ]]; then
  mkdir -p "$HOME/go/bin"
  # rm first: overwriting a binary in place makes macOS kill it (stale code-signature cache)
  rm -f "$HOME/go/bin/hist"
  cp bin/hist "$HOME/go/bin/hist"
  echo "installed ~/go/bin/hist"
fi
