#!/usr/bin/env bash
# Produce the embedded sandbox assets (bwrap binary + python-base rootfs) for a
# target architecture, so the binary can be built with `-tags sandbox_embed`
# (design §9.3: a self-contained binary that needs no external bwrap / rootfs).
#
# Usage:  scripts/build-sandbox-assets.sh [arm64|amd64]
# Output: pkg/sandbox/embedassets/linux_<arch>/{bwrap,python-base.tar.gz}
#
# - bwrap is built STATICALLY (musl + static libcap, selinux disabled), so the
#   host running n9e needs NO libcap/libselinux at all — truly self-contained.
# - python-base is python:3-slim with the package managers (apt/dpkg/pip) and
#   docs/tests/pycache stripped: smaller and no runtime installer (attack
#   surface). A self-test in the build fails fast if stripping broke python.
#
# Requires docker with the target platform (native on a matching host, emulated
# via binfmt otherwise) and network access for the alpine/python images +
# the bwrap source tarball. The assets are git-ignored.
set -euo pipefail

ARCH="${1:-$(go env GOARCH)}"
case "$ARCH" in
  arm64|amd64) ;;
  *) echo "unsupported arch: $ARCH (want arm64|amd64)" >&2; exit 1 ;;
esac
PLATFORM="linux/$ARCH"
BWRAP_VERSION="${BWRAP_VERSION:-0.11.0}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/pkg/sandbox/embedassets/linux_$ARCH"
mkdir -p "$OUT"

echo ">> [$ARCH] building STATIC bwrap $BWRAP_VERSION (alpine + static libcap)"
docker run --rm --platform "$PLATFORM" -e BWRAP_VERSION="$BWRAP_VERSION" -v "$OUT:/out" alpine:3.20 sh -euo pipefail -c '
  apk add --no-cache build-base meson ninja libcap-static libcap-dev linux-headers bash curl xz >/dev/null
  curl -fsSL "https://github.com/containers/bubblewrap/releases/download/v${BWRAP_VERSION}/bubblewrap-${BWRAP_VERSION}.tar.xz" -o /tmp/b.tar.xz
  tar -xJf /tmp/b.tar.xz -C /tmp
  cd "/tmp/bubblewrap-${BWRAP_VERSION}"
  meson setup build --prefer-static -Dselinux=disabled -Dman=disabled -Dc_link_args=-static >/dev/null
  ninja -C build bwrap >/dev/null
  cp build/bwrap /out/bwrap
  strip /out/bwrap || true
  chmod 0755 /out/bwrap
  echo "   $(file -b /out/bwrap)"
  ldd /out/bwrap 2>&1 | head -1 || true
'

echo ">> [$ARCH] building SLIM python-base (strip apt/dpkg/pip + docs/tests)"
docker build --platform "$PLATFORM" -t "n9e-pybase-$ARCH" -f - "$ROOT" >/dev/null <<'DOCKERFILE'
FROM python:3-slim
RUN set -eux; \
    for d in /usr/local/lib/python3.*; do \
      rm -rf "$d/ensurepip" "$d/lib2to3" "$d/idlelib" "$d/tkinter" "$d/test"; \
      find "$d" -type d \( -name test -o -name tests \) -exec rm -rf {} + 2>/dev/null || true; \
      rm -rf "$d"/site-packages/pip* "$d"/site-packages/setuptools* \
             "$d"/site-packages/pkg_resources "$d"/site-packages/_distutils_hack \
             "$d"/site-packages/wheel*; \
    done; \
    rm -rf /usr/lib/apt /etc/apt /var/lib/apt /var/cache/apt /var/lib/dpkg \
           /usr/bin/apt* /usr/bin/dpkg* /usr/sbin/dpkg* /usr/share/doc /usr/share/man \
           /usr/share/info /usr/share/locale; \
    find / -type d -name __pycache__ -prune -exec rm -rf {} + 2>/dev/null || true; \
    find / -name '*.pyc' -delete 2>/dev/null || true; \
    python3 -c "import ssl,json,socket,ctypes,hashlib,datetime,shutil; print('pybase self-test ok', __import__('sys').version.split()[0])"
DOCKERFILE
cid="$(docker create --platform "$PLATFORM" "n9e-pybase-$ARCH")"
docker export "$cid" | gzip -9 > "$OUT/python-base.tar.gz"
docker rm "$cid" >/dev/null
docker rmi "n9e-pybase-$ARCH" >/dev/null 2>&1 || true

echo ">> done:"
ls -lah "$OUT"
echo ">> build an embedded binary with:"
echo "   GOOS=linux GOARCH=$ARCH go build -tags sandbox_embed -o n9e-$ARCH ./cmd/center/main.go"
