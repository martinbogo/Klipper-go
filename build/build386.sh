#!/bin/bash
set -e

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)

export CGO_ENABLED=1
export GOARCH="386"

cd "$REPO_ROOT"
go mod download
go mod vendor
rm -rf ./chelper/libc_helper.so
rm -rf ./internal/pkg/chelper/libc_helper.so
rm -rf ./internal/pkg/chelper/libc_helper.a
./chelper/build_chelper_386.sh
cd "$REPO_ROOT/build"
go build -v -x -ldflags "-s -w" -o gklib ../cmd/gklib
