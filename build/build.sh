#!/bin/bash
set -e

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)

export GOOS="linux"
export CGO_ENABLED=1
export GOARCH="arm"
export CC=/home/liuxiaobo/gcc/arm-rockchip830-linux-uclibcgnueabihf/bin/arm-rockchip830-linux-uclibcgnueabihf-gcc

cd "$REPO_ROOT"
go mod download
go mod vendor
go mod tidy
rm -rf ./chelper/libc_helper.so
rm -rf ./internal/pkg/chelper/libc_helper.so
rm -rf ./internal/pkg/chelper/libc_helper.a
./chelper/build_chelper_arm.sh
cd "$REPO_ROOT/build"
go build -v -x -ldflags "-s -w" -o gklib ../cmd/gklib
