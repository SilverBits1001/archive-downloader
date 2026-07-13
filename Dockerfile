# Builds archive-downloader natively for linux/arm64 (same pattern as
# gabagool's own Dockerfile). Run via docker buildx with qemu, or let
# the GitHub Actions workflow drive it.
FROM --platform=linux/arm64 golang:1.25-bookworm AS build

RUN apt-get update && apt-get install -y \
    libsdl2-dev \
    libsdl2-ttf-dev \
    libsdl2-image-dev \
    libsdl2-gfx-dev

WORKDIR /build
COPY . .

# resolve dependency versions and generate go.sum, then build
RUN go mod tidy
RUN go build -v -o archive-downloader .

FROM scratch AS export
COPY --from=build /build/archive-downloader /archive-downloader
