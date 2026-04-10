#!/bin/bash
docker run --rm -v "$PWD":/usr/src/myapp -w /usr/src/myapp golang:latest /bin/bash -c "
apt-get update && apt-get install -y gcc-arm-linux-gnueabihf file libc6-dev-armhf-cross && \
git config --global --add safe.directory /usr/src/myapp && export CC=arm-linux-gnueabihf-gcc && \
export CGO_ENABLED=1 && \
export GOOS=linux && \
export GOARCH=arm && \
export GOARM=7 && \
./chelper/build_chelper_arm.sh && \
/usr/local/go/bin/go build -buildvcs=false -v -tags osusergo,netgo -ldflags \"-extldflags=-static -s -w\" -o gklib_static ./cmd/gklib
"
