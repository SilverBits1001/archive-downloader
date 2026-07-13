# Builds archive-downloader natively for linux/arm64.
# IMPORTANT: Debian bullseye base on purpose — its glibc (2.31) matches
# what the TrimUI Brick firmware ships. Newer bases (bookworm+) produce
# binaries the device's loader rejects. Same reason gabagool's own
# Dockerfile uses bullseye.
FROM --platform=linux/arm64 debian:bullseye AS build

RUN apt-get update && apt-get install -y \
    curl ca-certificates gcc libc6-dev \
    libsdl2-dev \
    libsdl2-ttf-dev \
    libsdl2-image-dev \
    libsdl2-gfx-dev

# Go 1.25 (arm64) — bullseye-era golang images don't have it
RUN curl -fsSLo /tmp/go.tgz https://go.dev/dl/go1.25.5.linux-arm64.tar.gz \
    && tar -C /usr/local -xzf /tmp/go.tgz && rm /tmp/go.tgz
ENV PATH="/usr/local/go/bin:${PATH}"

WORKDIR /build
COPY . .

# resolve dependency versions and generate go.sum, then build
RUN go mod tidy
RUN go build -v -o archive-downloader .

FROM scratch AS export
COPY --from=build /build/archive-downloader /archive-downloader
