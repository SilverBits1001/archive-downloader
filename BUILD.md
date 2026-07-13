# Building Archive Downloader (Go / gabagool rewrite)

Target: TrimUI Brick (tg5040), linux/arm64, cgo (SDL2 via gabagool).

## Option A - WSL2 Ubuntu cross-compile (what we use)

One-time setup inside WSL Ubuntu:

```sh
# arm64 package sources (Ubuntu keeps arm64 on ports.ubuntu.com)
sudo dpkg --add-architecture arm64
sudo tee /etc/apt/sources.list.d/arm64.list >/dev/null <<'EOF'
deb [arch=arm64] http://ports.ubuntu.com/ubuntu-ports noble main universe
deb [arch=arm64] http://ports.ubuntu.com/ubuntu-ports noble-updates main universe
EOF
sudo apt-get update
sudo apt-get install -y gcc-aarch64-linux-gnu pkg-config \
  libsdl2-dev:arm64 libsdl2-ttf-dev:arm64 \
  libsdl2-image-dev:arm64 libsdl2-gfx-dev:arm64

# Go >= 1.25 (Ubuntu's packaged Go is usually too old)
curl -LO https://go.dev/dl/go1.25.5.linux-amd64.tar.gz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.25.5.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
```

Then: `./build.sh` -> produces `pak/archive-downloader`.

## Option B - Docker (matches upstream gabagool builds)

gabagool's own Dockerfile builds natively in an arm64 container
(`FROM --platform=linux/arm64 golang:1.24-bullseye` + libsdl2*-dev),
relying on qemu/binfmt emulation. Slower, but zero cross-compile
quirks. See gabagool's repo `Dockerfile` + `build.sh` for the pattern.

## Packaging the pak

```
Archive Downloader.pak/
├── launch.sh            # from pak/launch.sh
├── archive-downloader   # built binary
├── config.yml           # user config (carries auth - do not distribute)
├── pak.json
├── lib/
│   └── libSDL2_gfx-1.0.so.0   # copy from Mortar/Pak Store pak (device lacks it)
└── res/
    ├── cacert.pem       # Mozilla CA bundle (SSL_CERT_FILE in launch.sh)
    └── arcade_names.tsv
```

Everything else (curl, jq, yq, 7zz, minui-*) is no longer needed:
HTTP/TLS, YAML, JSON, zip/7z/rar extraction and all UI are in the
binary.

## What the rewrite gains over the shell version

- Long filenames marquee-scroll automatically when highlighted
- True static download screen with per-file progress + speed
- Multi-select download queue (Select button, up to 2 concurrent)
- No flashing: the app owns the screen, no presenter relaunches
- Search, tag filters, cache + refresh, arcade naming, per-platform
  extraction rules, favorites, destinations - all ported
