# Builds archive-downloader natively for linux/arm64.
# quasimodo is the gabagool author's own toolchain image (Go + GCC +
# SDL2 dev libs) — the same base grout and other device-proven gabagool
# apps build from. Using it instead of a hand-rolled base avoids
# glibc/SDL version drift against real handheld firmware.
FROM --platform=linux/arm64 ghcr.io/brandonkowalski/quasimodo:latest AS build

WORKDIR /build
COPY . .

# resolve dependency versions and generate go.sum, then build
RUN go mod tidy
RUN go build -v -o archive-downloader .

FROM scratch AS export
COPY --from=build /build/archive-downloader /archive-downloader
