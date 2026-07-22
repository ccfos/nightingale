#!/usr/bin/env bash
# Stage the pinned categraf release tarballs into agents/categraf/, so the n9e
# release archive ships a collector that hosts can install with one command even
# on air-gapped / intranet deployments (doc/design/categraf-install).
#
# Usage:  scripts/download_categraf.sh
# Output: agents/categraf/categraf-linux-{amd64,arm64}.tar.gz
#
# - The version is read from center/router/agentassets/categraf.version, the
#   SAME file that is go:embed'ed into the binary. Bumping categraf is a
#   one-line edit there; the served bytes and the reported version cannot drift.
# - Both architectures are always fetched: a monitored host's architecture is
#   independent of the n9e server's, and an intranet deployment has no CDN to
#   fall back to.
# - Tarballs are stored under version-less names so the serving handler maps
#   arch -> constant filename and never has to glob or parse.
#
# Invoked from .goreleaser.yaml's before.hooks (release time only, including
# snapshot builds — archives.files errors out if the glob matches nothing).
# Deliberately NOT wired into `make build`: a local compile has no reason to
# pull ~110MB. Run it by hand when you want a local server to serve the
# install endpoint. Requires curl and network access to github.com.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PIN_FILE="$ROOT/center/router/agentassets/categraf.version"
OUT="$ROOT/agents/categraf"

[ -f "$PIN_FILE" ] || { echo "version pin not found: $PIN_FILE" >&2; exit 1; }
CATEGRAF_VERSION="$(tr -d '[:space:]' < "$PIN_FILE")"
[ -n "$CATEGRAF_VERSION" ] || { echo "version pin is empty: $PIN_FILE" >&2; exit 1; }

BASE_URL="https://github.com/flashcatcloud/categraf/releases/download/${CATEGRAF_VERSION}"
STAMP="$OUT/.staged-version"

# Fast path: already staged at the pinned version. Required, not just a nicety —
# `make release` runs the before-hooks on every snapshot build, and without this
# every local release rehearsal would re-download ~110MB.
if [ -f "$STAMP" ] && [ "$(cat "$STAMP")" = "$CATEGRAF_VERSION" ] &&
    [ -s "$OUT/categraf-linux-amd64.tar.gz" ] && [ -s "$OUT/categraf-linux-arm64.tar.gz" ]; then
    echo ">> categraf $CATEGRAF_VERSION already staged in $OUT, skipping"
    exit 0
fi

if command -v sha256sum >/dev/null 2>&1; then
    SHA_CHECK="sha256sum -c"
elif command -v shasum >/dev/null 2>&1; then
    SHA_CHECK="shasum -a 256 -c"
else
    echo "neither sha256sum nor shasum found, cannot verify downloads" >&2
    exit 1
fi

mkdir -p "$OUT"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo ">> downloading categraf $CATEGRAF_VERSION checksums"
curl -fsSL -o "$TMP/checksums.txt" "$BASE_URL/checksums.txt" \
    || { echo "failed to download checksums.txt from $BASE_URL" >&2; exit 1; }

for ARCH in amd64 arm64; do
    ASSET="categraf-${CATEGRAF_VERSION}-linux-${ARCH}.tar.gz"

    echo ">> [$ARCH] downloading $ASSET"
    curl -fsSL -o "$TMP/$ASSET" "$BASE_URL/$ASSET" \
        || { echo "failed to download $ASSET from $BASE_URL" >&2; exit 2; }

    # Verification runs inside $TMP because checksums.txt names the upstream
    # files, not the version-less names we save them under.
    echo ">> [$ARCH] verifying checksum"
    ( cd "$TMP" && grep " ${ASSET}\$" checksums.txt | $SHA_CHECK - ) \
        || { echo "checksum verification failed for $ASSET" >&2; exit 3; }

    mv -f "$TMP/$ASSET" "$OUT/categraf-linux-${ARCH}.tar.gz"
done

echo "$CATEGRAF_VERSION" > "$STAMP"
echo ">> categraf $CATEGRAF_VERSION staged in $OUT"
