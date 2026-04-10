FROM golang:1.25-bookworm

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        gcc-arm-linux-gnueabihf \
        file \
        libc6-dev-armhf-cross \
    && rm -rf /var/lib/apt/lists/*
