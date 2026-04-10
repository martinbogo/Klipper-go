GO = go
APP = gklib
BUILD_DIR = build
APP_BIN = $(BUILD_DIR)/$(APP)
RUN_OPTIONS = -I /dev/ttyUSB0 -a /home/aston/klippy_uds1 ~/printer_data/config/printer.cfg
DLV = dlv
GCC = gcc
ARMROOT = $(PWD)/rv1106-cross-compilation-toolchain/arm-rockchip830-linux-uclibcgnueabihf
ARMCC = $(ARMROOT)/bin/arm-rockchip830-linux-uclibcgnueabihf-gcc
ARMLIB = $(ARMROOT)/arm-rockchip830-linux-uclibcgnueabihf/sysroot/lib
ARMLIBCHELPER = $(PWD)/chelper/build_chelper_arm.sh
386LIBCHELPER = $(PWD)/chelper/build_chelper_386.sh
64LIBCHELPER = $(PWD)/chelper/build_chelper_x64.sh

.PHONY: build run tools tidy debug vet clean

build:export GOOS=linux
build:export GOARCH=amd64
build:export CGO_ENABLED=1
build:
	$(64LIBCHELPER)
	$(GO) build -v -o $(APP_BIN) ./cmd/gklib

build-arm:export GOOS=linux
build-arm:export GOARCH=arm
build-arm:export CGO_ENABLED=1
build-arm:export LD_LIBRARY_PATH=$(LIB)
build-arm:export CC=${ARMCC}
build-arm:
	$(ARMLIBCHELPER)
	$(GO) build -o $(APP_BIN) ./cmd/gklib

build-386:export GOOS=linux
build-386:export GOARCH=386
build-386:export CGO_ENABLED=1
build-386:export CC=$(GCC)
build-386:
	$(386LIBCHELPER)
	$(GO) build -o $(APP_BIN) ./cmd/gklib

build-amd64:export GOOS=linux
build-amd64:export GOARCH=amd64
build-amd64:export CGO_ENABLED=1
build-amd64:export CC=$(GCC)
build-amd64:
	$(64LIBCHELPER)
	$(GO) build -o $(APP_BIN) ./cmd/gklib

build-armlibchelper:
	$(ARMLIBCHELPER)

build-386libchelper:
	$(386LIBCHELPER)

build-amd64libchelper:
	$(64LIBCHELPER)

run:export LD_LIBRARY_PATH=internal/pkg/chelper
run:
	./$(APP_BIN) $(RUN_OPTIONS)

debug:
	$(DLV) exec ./$(APP_BIN) -- $(RUN_OPTIONS)

tidy:
	$(GO) mod tidy

vet:
	-$(GO) vet gklipper/klippy

vendor:
	$(GO) mod vendor

tools:
	$(GO) install github.com/go-delve/delve/cmd/dlv@v1.7.3

clean:
	rm -rf $(APP_BIN)
	rm -rf cmd/config_convert/config_convert
	rm -rf internal/pkg/chelper/libc_helper.so
	rm -rf internal/pkg/chelper/libc_helper.a

buildconfigconvert:
	$(GO) build -o cmd/config_convert/config_convert ./cmd/config_convert

buildconfigconvert32:export GOOS=linux
buildconfigconvert32:export GOARCH=arm
buildconfigconvert32:export CGO_ENABLED=1
buildconfigconvert32:export CC=${ARMCC}
buildconfigconvert32:
	$(GO) build -o cmd/config_convert/config_convert ./cmd/config_convert

configconvert:
	cmd/config_convert/config_convert --i=/home/aston/printer_data/config/printer.cfg --o=printer.bc -p=false

run64: build-amd64libchelper build-amd64 run
run32: build-386libchelper build-386 run
