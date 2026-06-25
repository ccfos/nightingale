#!/usr/bin/env bash
# Produce the embedded sandbox assets (bwrap binary + python-base rootfs) for a
# target architecture, so the binary can be built with `-tags sandbox_embed`
# (design §9.3: a self-contained binary that needs no external bwrap / rootfs).
#
# Usage:  scripts/build-sandbox-assets.sh [arm64|amd64]
# Output: pkg/sandbox/embedassets/linux_<arch>/{bwrap,python-base.tar.gz}
#
# Requires docker (with the target platform available — native on a matching
# host, emulated via binfmt otherwise). The assets are git-ignored.
#
# NOTE: the bwrap extracted from the distro package is dynamically linked, so it
# still needs its libs (libcap…) on the HOST that runs n9e. For a fully self-
# contained binary, swap in a statically-linked bwrap here — a follow-up.
set -euo pipefail

ARCH="${1:-$(go env GOARCH)}"
case "$ARCH" in
  arm64|amd64) ;;
  *) echo "unsupported arch: $ARCH (want arm64|amd64)" >&2; exit 1 ;;
esac
PLATFORM="linux/$ARCH"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/pkg/sandbox/embedassets/linux_$ARCH"
mkdir -p "$OUT"

echo ">> [$ARCH] extracting bwrap from ubuntu:24.04"
docker run --rm --platform "$PLATFORM" -v "$OUT:/out" ubuntu:24.04 bash -c '
  apt-get update -qq >/dev/null && apt-get install -y -qq bubblewrap >/dev/null
  cp /usr/bin/bwrap /out/bwrap && chmod 0755 /out/bwrap'

echo ">> [$ARCH] exporting python:3-slim rootfs -> python-base.tar.gz"
cid="$(docker create --platform "$PLATFORM" python:3-slim)"
docker export "$cid" | gzip -9 > "$OUT/python-base.tar.gz"
docker rm "$cid" >/dev/null

echo ">> done:"
ls -lah "$OUT"
echo ">> build an embedded binary with:"
echo "   GOOS=linux GOARCH=$ARCH go build -tags sandbox_embed -o n9e-$ARCH ./cmd/center/main.go"
