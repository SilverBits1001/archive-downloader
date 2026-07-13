#!/bin/sh
PAK_DIR="$(dirname "$0")"
cd "$PAK_DIR" || exit 1

export LD_LIBRARY_PATH=/usr/trimui/lib:$PAK_DIR/lib:$LD_LIBRARY_PATH
# Go's TLS stack reads the CA bundle from this env var on Linux
export SSL_CERT_FILE="$PAK_DIR/res/cacert.pem"

# stdout/stderr to a log so loader errors (missing libs etc.) are visible
mkdir -p "$PAK_DIR/res"
./archive-downloader > "$PAK_DIR/res/launch-stdout.log" 2>&1
