#!/usr/bin/env bash
# Native fallback when the Docker builder image cannot be downloaded.
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."
export GOTOOLCHAIN=local CGO_ENABLED=1
version="$(go env GOVERSION)"
if [[ "$version" != go1.26.1 ]]; then echo 'Go 1.26.1 is required for release builds.' >&2; exit 1; fi
case "$(go env GOOS)/$(go env GOARCH)" in linux/amd64|linux/arm64) ;; *) echo 'Supported platforms: Linux amd64/arm64' >&2; exit 1;; esac
getconf GNU_LIBC_VERSION
arch="$(go env GOARCH)"
mkdir -p "build/linux/$arch"
go build -buildvcs=false -trimpath -buildmode=c-shared -ldflags='-s -w' -o "build/linux/$arch/model-quality.so" .
python3 scripts/package.py --arch "$arch"
