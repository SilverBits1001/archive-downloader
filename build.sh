#!/bin/sh
# Cross-compile archive-downloader for the TrimUI Brick (linux/arm64)
# from a WSL2 Ubuntu (or any Debian/Ubuntu amd64) host.
#
# One-time setup (see BUILD.md):
#   sudo dpkg --add-architecture arm64
#   (add ports.ubuntu.com arm64 sources)
#   sudo apt-get update
#   sudo apt-get install -y gcc-aarch64-linux-gnu \
#     libsdl2-dev:arm64 libsdl2-ttf-dev:arm64 \
#     libsdl2-image-dev:arm64 libsdl2-gfx-dev:arm64
#   (install Go >= 1.25 from go.dev)
set -e
cd "$(dirname "$0")"

export CGO_ENABLED=1
export GOOS=linux
export GOARCH=arm64
export CC=aarch64-linux-gnu-gcc
export PKG_CONFIG_PATH=/usr/lib/aarch64-linux-gnu/pkgconfig

go build -o pak/archive-downloader .
echo "Built pak/archive-downloader ($(du -h pak/archive-downloader | cut -f1))"
