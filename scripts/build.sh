#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."
arch="$(uname -m)"
case "$arch" in x86_64) arch=amd64;; aarch64|arm64) arch=arm64;; *) echo 'Supported platforms: Linux amd64/arm64' >&2; exit 1;; esac
builder="cpa-model-quality-builder:go1.26.1-glibc"
docker build --pull=false -f scripts/Dockerfile.builder -t "$builder" scripts
mkdir -p build
docker run --rm -v "$PWD:/src" -e GOCACHE=/src/build/.gocache-glibc -e GOPATH=/src/build/.gopath "$builder" sh -c \
  "go build -buildvcs=false -trimpath -buildmode=c-shared -ldflags='-s -w' -o build/linux/$arch/model-quality.so . && chown -R $(id -u):$(id -g) /src/build"
python3 scripts/package.py --arch "$arch"
