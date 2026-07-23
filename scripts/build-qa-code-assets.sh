#!/usr/bin/env bash
# Produce the QA code corpus assets (filtered source snapshots of nightingale /
# categraf / n9e-fe as tar.gz + manifest.json), so the center binary can be
# built with `-tags qa_code_embed` and the doc-qa skill can grep real code via
# the list_code / search_code / read_code builtin tools.
#
# Usage:  scripts/build-qa-code-assets.sh
# Output: aiagent/skill/embedded/codeassets/{n9e,categraf,fe}.tar.gz + manifest.json
#
# Source resolution:
#   - n9e:      always the current checkout (git ls-files @ HEAD) — the corpus
#               is guaranteed to match the binary being released.
#   - categraf: $CATEGRAF_LOCAL (a local checkout) if set, else the source
#               tarball of the latest GitHub release tag ($CATEGRAF_REF pins it).
#   - fe:       $FE_LOCAL if set, else latest GitHub release tag ($FE_REF pins
#               it). Note fe.sh downloads the *build artifact* of the same
#               latest tag, so the corpus matches the embedded front-end UI.
#
# Filters (hard lines: fe src/plus is commercial code, n9e front/ is the 17MB
# statik blob — neither may ever enter the corpus):
#   - n9e:      *.go *.toml *.md minus vendor/ front/ pub/ integrations/ docker/
#   - categraf: *.go *.toml *.md minus vendor/
#   - fe:       src/**/*.{ts,tsx} minus src/plus/
#   - all:      minus test files (_test.go / testdata/ / *.test|spec.ts(x) /
#               __tests__/ __mocks__/) — the corpus is the doc-qa skill's
#               fact source for exact identifiers, and test files are full of
#               mock constants and fake fixtures that would be taken as real
#
# The assets are git-ignored; ~4.6MB total. GITHUB_TOKEN is used when set to
# avoid API rate limits in CI.
set -euo pipefail

# Keep macOS bsdtar from adding ._* AppleDouble noise entries.
export COPYFILE_DISABLE=1

# Test-file exclusion regexes (see the filter note in the header).
GO_TEST_FILTER='(_test\.go$|(^|/)testdata/)'
FE_TEST_FILTER='(\.(test|spec)\.tsx?$|(^|/)(__tests__|__mocks__)/)'

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/aiagent/skill/embedded/codeassets"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
mkdir -p "$OUT"

# gh_api [curl-args...] <url> — GitHub 请求统一出口，带瞬时错误重试
# （curl --retry 覆盖超时/429/5xx），API 限流场景优先用 GITHUB_TOKEN。
gh_api() {
  if [ -n "${GITHUB_TOKEN:-}" ]; then
    curl -fsSL --retry 3 --retry-delay 5 -H "Authorization: Bearer $GITHUB_TOKEN" "$@"
  else
    curl -fsSL --retry 3 --retry-delay 5 "$@"
  fi
}

# latest_tag <owner/repo> — resolve the latest release tag (same awk as fe.sh).
latest_tag() {
  gh_api "https://api.github.com/repos/$1/releases/latest" | awk '/tag_name/{print $4;exit}' FS='[""]'
}

# tag_commit <owner/repo> <tag> — resolve a tag to its commit sha (best-effort).
tag_commit() {
  gh_api "https://api.github.com/repos/$1/commits/$2" 2>/dev/null | awk '/"sha"/{print $4;exit}' FS='[""]' || echo unknown
}

# fetch_source <owner/repo> <ref> <dest> — download + unpack a source tarball,
# leaving the repo content directly under <dest> (top-level dir stripped).
fetch_source() {
  local repo="$1" ref="$2" dest="$3"
  local tarball="$WORK/${repo##*/}-src.tar.gz"
  mkdir -p "$dest"
  echo ">> downloading $repo @ $ref"
  # 先落盘再解包：直接管进 tar 的话，curl 中途重试会把重发的字节混进 tar
  # 流造成静默损坏；落盘后若仍损坏由 tar 报错兜底（workflow 层还有整脚本
  # 级重试）。
  gh_api -o "$tarball" "https://github.com/$repo/archive/refs/tags/$ref.tar.gz"
  tar xzf "$tarball" -C "$dest" --strip-components 1
}

# stage_files <srcdir> <stagedir> <filelist-on-stdin> — copy the listed
# relative paths from srcdir into stagedir via a tar pipe (portable, keeps
# subdirs, no GNU cp --parents dependency).
stage_files() {
  local src="$1" stage="$2"
  mkdir -p "$stage"
  (cd "$src" && tar cf - -T -) | (cd "$stage" && tar xf -)
}

# pack <stagedir> <out.tar.gz> — deterministic-ish flat tarball (sorted file
# list, no top-level dir; entries extract straight into code/<repo>/).
pack() {
  local stage="$1" out="$2"
  (cd "$stage" && find . -type f | sed 's|^\./||' | LC_ALL=C sort | tar czf "$out" -T -)
}

count_files() { find "$1" -type f | wc -l | tr -d ' '; }

# tree_overview <stagedir> — emit a 2-level directory listing.
tree_overview() {
  (cd "$1" && find . -maxdepth 2 -type d | sed 's|^\./||;/^\.$/d' | LC_ALL=C sort | sed 's|^|- |')
}

# ---------------------------------------------------------------------------
# n9e — current checkout
# ---------------------------------------------------------------------------
N9E_STAGE="$WORK/n9e"
N9E_COMMIT="$(git -C "$ROOT" rev-parse HEAD)"
N9E_REF="$(git -C "$ROOT" describe --tags --abbrev=0 2>/dev/null || git -C "$ROOT" rev-parse --abbrev-ref HEAD)"
echo ">> staging n9e @ $N9E_REF ($N9E_COMMIT)"
git -C "$ROOT" -c core.quotePath=false ls-files '*.go' '*.toml' '*.md' \
  | grep -vE '^(vendor|front|pub|integrations|docker)/' \
  | grep -vE "$GO_TEST_FILTER" \
  | stage_files "$ROOT" "$N9E_STAGE"

cat > "$N9E_STAGE/TREE.md" <<EOF
# nightingale (n9e) code corpus — $N9E_REF

Server source of the Nightingale monitoring platform. Key directories:

- models/ — GORM data models and constants (severity labels, table fields, DB2FE/FE2DB conversion)
- center/router/ — HTTP API routes (paths, auth headers, request/response shapes)
- alert/ — alert engine (rule evaluation, event pipeline, notification dispatch)
- pushgw/ — metrics ingestion gateway (remote write, ident/host handling)
- memsto/ — in-memory caches synced from DB
- etc/ — sample config files (config.toml and friends, with default values)
- cmd/ — process entrypoints (center / edge / alert / pushgw / cli)
- aiagent/ — AI assistant runtime (skills, builtin tools)
- pkg/ — shared libraries

## Directory overview (2 levels)

$(tree_overview "$N9E_STAGE")
EOF
pack "$N9E_STAGE" "$OUT/n9e.tar.gz"

# ---------------------------------------------------------------------------
# categraf — local checkout or latest release source tarball
# ---------------------------------------------------------------------------
CAT_STAGE="$WORK/categraf"
if [ -n "${CATEGRAF_LOCAL:-}" ]; then
  CAT_SRC="$CATEGRAF_LOCAL"
  CAT_COMMIT="$(git -C "$CAT_SRC" rev-parse HEAD 2>/dev/null || echo unknown)"
  CAT_REF="$(git -C "$CAT_SRC" describe --tags --abbrev=0 2>/dev/null || echo local)"
  echo ">> staging categraf from $CAT_SRC @ $CAT_REF"
  git -C "$CAT_SRC" -c core.quotePath=false ls-files '*.go' '*.toml' '*.md' \
    | grep -v '^vendor/' \
    | grep -vE "$GO_TEST_FILTER" \
    | stage_files "$CAT_SRC" "$CAT_STAGE"
else
  CAT_REF="${CATEGRAF_REF:-$(latest_tag flashcatcloud/categraf)}"
  [ -n "$CAT_REF" ] || { echo "failed to resolve categraf latest tag" >&2; exit 1; }
  CAT_SRC="$WORK/categraf-src"
  fetch_source flashcatcloud/categraf "$CAT_REF" "$CAT_SRC"
  CAT_COMMIT="$(tag_commit flashcatcloud/categraf "$CAT_REF")"
  (cd "$CAT_SRC" && find . -type f \( -name '*.go' -o -name '*.toml' -o -name '*.md' \) -not -path './vendor/*' | sed 's|^\./||') \
    | grep -vE "$GO_TEST_FILTER" \
    | stage_files "$CAT_SRC" "$CAT_STAGE"
fi

cat > "$CAT_STAGE/TREE.md" <<EOF
# categraf code corpus — $CAT_REF

Source of the categraf collection agent. Key directories:

- inputs/ — one directory per collection plugin; plugin Go code defines the
  exact metric names and fields, README.md documents them, and the sample
  config for plugin X lives in conf/input.X/X.toml
- conf/ — sample configs: config.toml (agent global, [heartbeat], [[writers]])
  and input.*/ per-plugin instance samples ([[instances]] syntax)
- config/ — global config structs, defaults and environment variables
- agent/ — collection scheduling
- writer/ — remote-write senders
- house/ — metrics processing helpers

## Directory overview (2 levels)

$(tree_overview "$CAT_STAGE")
EOF
pack "$CAT_STAGE" "$OUT/categraf.tar.gz"

# ---------------------------------------------------------------------------
# fe — local checkout or latest release source tarball (NEVER src/plus)
# ---------------------------------------------------------------------------
FE_STAGE="$WORK/fe"
if [ -n "${FE_LOCAL:-}" ]; then
  FE_SRC="$FE_LOCAL"
  FE_COMMIT="$(git -C "$FE_SRC" rev-parse HEAD 2>/dev/null || echo unknown)"
  FE_REF_RESOLVED="$(git -C "$FE_SRC" describe --tags --abbrev=0 2>/dev/null || echo local)"
  echo ">> staging fe from $FE_SRC @ $FE_REF_RESOLVED"
  # git pathspec 通配对 ** 不可靠（无 :(glob) 魔法时语义漂移），用 grep 过滤
  git -C "$FE_SRC" -c core.quotePath=false ls-files src \
    | grep -E '\.(ts|tsx)$' \
    | grep -v '^src/plus/' \
    | grep -vE "$FE_TEST_FILTER" \
    | stage_files "$FE_SRC" "$FE_STAGE"
else
  FE_REF_RESOLVED="${FE_REF:-$(latest_tag n9e/fe)}"
  [ -n "$FE_REF_RESOLVED" ] || { echo "failed to resolve fe latest tag" >&2; exit 1; }
  FE_SRC="$WORK/fe-src"
  fetch_source n9e/fe "$FE_REF_RESOLVED" "$FE_SRC"
  FE_COMMIT="$(tag_commit n9e/fe "$FE_REF_RESOLVED")"
  (cd "$FE_SRC" && find ./src -type f \( -name '*.ts' -o -name '*.tsx' \) -not -path './src/plus/*' | sed 's|^\./||') \
    | grep -vE "$FE_TEST_FILTER" \
    | stage_files "$FE_SRC" "$FE_STAGE"
fi

# Belt and braces: the commercial plus tree must never be packed.
rm -rf "$FE_STAGE/src/plus"

cat > "$FE_STAGE/TREE.md" <<EOF
# n9e-fe code corpus — $FE_REF_RESOLVED

Front-end (React/TypeScript) source of the Nightingale web UI. Key directories:

- src/locales/ — zh_CN/en_US UI copy: menu names, form labels, hints. The
  authoritative source for "what is this called in the UI / where to click"
- src/pages/ — one directory per feature page (dashboard, alertRules,
  explorer, notificationRules, ...): form fields, validation, interactions
- src/components/ — shared UI components
- src/services/ — HTTP API calls made by the UI (paths and payloads)
- src/routers/ — front-end route table (URL path -> page)

## Directory overview (2 levels)

$(tree_overview "$FE_STAGE")
EOF
pack "$FE_STAGE" "$OUT/fe.tar.gz"

# ---------------------------------------------------------------------------
# manifest
# ---------------------------------------------------------------------------
GENERATED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
cat > "$OUT/manifest.json" <<EOF
[
  {"repo": "n9e", "ref": "$N9E_REF", "commit": "$N9E_COMMIT", "files": $(count_files "$N9E_STAGE"), "generated_at": "$GENERATED_AT"},
  {"repo": "categraf", "ref": "$CAT_REF", "commit": "$CAT_COMMIT", "files": $(count_files "$CAT_STAGE"), "generated_at": "$GENERATED_AT"},
  {"repo": "fe", "ref": "$FE_REF_RESOLVED", "commit": "$FE_COMMIT", "files": $(count_files "$FE_STAGE"), "generated_at": "$GENERATED_AT"}
]
EOF

echo ">> done:"
ls -la "$OUT"
