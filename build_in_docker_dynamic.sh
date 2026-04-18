#!/bin/bash
set -euo pipefail

docker run --rm -v "$PWD":/usr/src/myapp -w /usr/src/myapp golang:1.25-bookworm /bin/bash -lc '
export PATH=/usr/local/go/bin:$PATH
export CC="$PWD/rv1106-cross-compilation-toolchain/arm-rockchip830-linux-uclibcgnueabihf/bin/arm-rockchip830-linux-uclibcgnueabihf-gcc"
export CGO_ENABLED=1
export GOOS=linux
export GOARCH=arm
export GOARM=7
./chelper/build_chelper_arm.sh
/usr/local/go/bin/go build -buildvcs=false -ldflags="-s -w" -o gklib_uclibc_current ./cmd/gklib
'
