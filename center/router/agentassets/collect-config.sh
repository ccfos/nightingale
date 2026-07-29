#!/usr/bin/env bash
# Categraf collect-plugin config applier, served by nightingale.
#
#   curl -fsSL '<n9e>/api/n9e/agents/categraf/collect.sh' \
#       | sudo bash -s -- --input mysql --conf-b64 '<base64 of the toml>'
#
# Unlike install-categraf.sh.tmpl this file is static: the config content
# travels inside the command line (base64), so nothing server-side is rendered
# into it and there is no injection surface to reason about.
#
# Flags REQUIRE the `bash -s --` form; plain `| sudo bash --input x` silently
# drops them. This script never calls `read`: when piped, stdin IS the script,
# so any prompt would consume its own source.
set -euo pipefail

INSTALL_DIR=""
INPUT=""
CONF_B64=""
SKIP_TEST=0
NO_RESTART=0
TEST_TIMEOUT=15

log()  { printf '\033[0;32m[categraf]\033[0m %s\n' "$*"; }
warn() { printf '\033[0;33m[categraf]\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[0;31m[categraf] ERROR:\033[0m %s\n' "$*" >&2; exit 1; }

usage() {
    cat <<'USAGE'
Usage: curl -fsSL <url>/api/n9e/agents/categraf/collect.sh | sudo bash -s -- [options]

  --input <name>        collect plugin name, e.g. mysql (writes conf/input.<name>/<name>.toml)
  --conf-b64 <base64>   base64-encoded toml content for that plugin (required)
  --dir <path>          categraf install directory (default: auto-detected from
                        the systemd unit, falling back to /opt/categraf)
  --test-timeout <sec>  how long to let `categraf --test` run (default: 15)
  --skip-test           write the config without test-running the plugin first
  --no-restart          write (and test) only; do not restart the categraf service
  -h, --help            show this help
USAGE
}

# Guard every value-taking option: without the check, a trailing "--dir" with no
# value makes `shift 2` fail, and under `set -e` the script dies silently with no
# message at all — the one failure path that would tell the user nothing.
need_value() { [ "$2" -ge 2 ] || die "$1 requires a value (use --help)"; }

while [ $# -gt 0 ]; do
    case "$1" in
        --input) need_value "$1" $#; INPUT="$2"; shift 2 ;;
        --conf-b64) need_value "$1" $#; CONF_B64="$2"; shift 2 ;;
        --dir) need_value "$1" $#; INSTALL_DIR="$2"; shift 2 ;;
        --test-timeout) need_value "$1" $#; TEST_TIMEOUT="$2"; shift 2 ;;
        --skip-test) SKIP_TEST=1; shift ;;
        --no-restart) NO_RESTART=1; shift ;;
        -h|--help) usage; exit 0 ;;
        *) die "unknown option: $1 (use --help)" ;;
    esac
done

[ -n "$INPUT" ] || die "--input is required (use --help)"
[ -n "$CONF_B64" ] || die "--conf-b64 is required (use --help)"

# The plugin name becomes a path component (conf/input.<name>/), so constrain it
# to what categraf plugin names actually use. This is what makes traversal via
# --input '../..' impossible by construction.
printf '%s' "$INPUT" | grep -Eq '^[a-z0-9_]{1,64}$' \
    || die "invalid --input '$INPUT' (want lowercase letters, digits, underscore)"

printf '%s' "$TEST_TIMEOUT" | grep -Eq '^[0-9]{1,3}$' \
    || die "--test-timeout expects seconds as a plain number"

# ---------------------------------------------------------------- preflight --
[ "$(id -u)" -eq 0 ] || die "must run as root (prefix the command with sudo)"
[ "$(uname -s)" = "Linux" ] || die "this script supports Linux only"

# ------------------------------------------------- locate the installation ---
# Order: --dir > the systemd unit's WorkingDirectory (categraf --install derives
# it from the binary location, so it is authoritative for one-click installs
# that used a custom --dir) > the default install path.
if [ -z "$INSTALL_DIR" ] && command -v systemctl >/dev/null 2>&1; then
    # `|| true` inside the substitution: systemctl show exits non-zero when the
    # unit does not exist (systemd >= 246) or the system was not booted with
    # systemd; under set -e + pipefail that would kill the whole script with no
    # output before the /opt/categraf fallback below could apply.
    wd="$(systemctl show -p WorkingDirectory categraf 2>/dev/null | sed 's/^WorkingDirectory=//' || true)"
    case "$wd" in
        /*) [ -x "$wd/categraf" ] && INSTALL_DIR="$wd" ;;
    esac
fi
INSTALL_DIR="${INSTALL_DIR:-/opt/categraf}"

# Absolute from here on: the test step cd's into $INSTALL_DIR and later paths
# would otherwise resolve a second time against it.
case "$INSTALL_DIR" in
    /*) ;;
    *) INSTALL_DIR="$PWD/$INSTALL_DIR" ;;
esac
INSTALL_DIR="${INSTALL_DIR%/}"

[ -x "$INSTALL_DIR/categraf" ] \
    || die "categraf not found in $INSTALL_DIR; install it first, or pass --dir <path>"
[ -d "$INSTALL_DIR/conf" ] \
    || die "$INSTALL_DIR/conf does not exist; is this a categraf installation?"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# ------------------------------------------------------------- decode --------
# `|| die` on the same line: under `set -e` with a pipeline, a base64 failure
# would otherwise kill the script with no message. --decode is spelled -d for
# busybox compatibility; whitespace/newlines inside the base64 are tolerated.
printf '%s' "$CONF_B64" | base64 -d > "$TMP/new.toml" 2>/dev/null \
    || die "--conf-b64 is not valid base64"
[ -s "$TMP/new.toml" ] || die "decoded config is empty"

# The decoded bytes are written to a config file and never executed, but they
# will be parsed as TOML by a root service — refuse binary junk early so a
# copy-paste accident fails here with a clear message instead of at parse time.
if command -v file >/dev/null 2>&1; then
    case "$(file -b --mime-type "$TMP/new.toml")" in
        text/*|application/toml|inode/x-empty) ;;
        *) die "decoded config does not look like text; re-copy the command from nightingale" ;;
    esac
fi

# ------------------------------------------------------------- install -------
PLUGIN_DIR="$INSTALL_DIR/conf/input.$INPUT"
TARGET="$PLUGIN_DIR/$INPUT.toml"

# WROTE guards the failure-cleanup below: when the file already had exactly
# this content nothing was written, and a failing test run (service briefly
# down, say) must NOT delete or roll back the user's working config.
WROTE=0
BACKUP=""
if [ -f "$TARGET" ] && cmp -s "$TMP/new.toml" "$TARGET"; then
    log "$TARGET already has exactly this content; nothing to write"
    # Fall through to test/restart anyway: the user may be re-running the
    # command precisely because the service never picked the file up.
else
    mkdir -p "$PLUGIN_DIR"
    WROTE=1
    if [ -f "$TARGET" ]; then
        BACKUP="$TARGET.bak-$(date +%Y%m%d%H%M%S)"
        cp -a "$TARGET" "$BACKUP"
        log "existing config backed up to $BACKUP"
    fi
    # Write-then-rename so a cut connection can never leave a half-written
    # config for the service to load. 600: collect configs routinely hold
    # database credentials.
    install -m 600 "$TMP/new.toml" "$TARGET.n9e-tmp"
    mv -f "$TARGET.n9e-tmp" "$TARGET"
    log "wrote $TARGET"
fi

# --------------------------------------------------------------- test --------
# `categraf --test` runs the plugin for real and prints one sample per line,
# each starting with a unix timestamp — but it never exits on its own, so it is
# run under a timeout and judged by its output, not its exit code. Config
# errors also do not change the exit code (categraf only logs "E! ..."), which
# is one more reason the sample count is the only trustworthy signal.
if [ "$SKIP_TEST" -eq 0 ]; then
    log "test-running the '$INPUT' plugin (up to ${TEST_TIMEOUT}s) ..."
    TEST_OUT="$TMP/test.out"
    if command -v timeout >/dev/null 2>&1; then
        (cd "$INSTALL_DIR" && timeout "$TEST_TIMEOUT" ./categraf --test --inputs "$INPUT" >"$TEST_OUT" 2>&1) || true
    else
        # No coreutils timeout: background + kill. `wait` returns the killed
        # process's status, hence the `|| true`.
        (cd "$INSTALL_DIR" && ./categraf --test --inputs "$INPUT" >"$TEST_OUT" 2>&1) &
        TPID=$!
        sleep "$TEST_TIMEOUT"
        kill "$TPID" 2>/dev/null || true
        wait "$TPID" 2>/dev/null || true
    fi

    SAMPLES="$(grep -Ec '^[0-9]{9,} ' "$TEST_OUT" || true)"
    if [ "$SAMPLES" -gt 0 ]; then
        log "plugin produced $SAMPLES sample(s); first few:"
        grep -E '^[0-9]{9,} ' "$TEST_OUT" | head -5 | sed 's/^/    /'
        # Samples and errors can coexist (e.g. one of two instances is
        # unreachable) — surface the errors instead of hiding them behind the
        # green line above.
        if grep -q ' E! ' "$TEST_OUT"; then
            warn "the test run also reported errors:"
            grep ' E! ' "$TEST_OUT" | head -5 | sed 's/^/    /' >&2
        fi
    else
        warn "the test run produced no samples; plugin output follows:"
        tail -20 "$TEST_OUT" | sed 's/^/    /' >&2
        if [ "$WROTE" -eq 1 ]; then
            if [ -n "$BACKUP" ]; then
                cp -a "$BACKUP" "$TARGET"
                warn "previous config restored from $BACKUP"
            else
                rm -f "$TARGET"
                warn "new config removed again"
            fi
        fi
        die "check the connection parameters and re-copy the command from nightingale (or rerun with --skip-test to force)"
    fi
fi

# -------------------------------------------------------------- restart ------
# categraf does not reload plugin configs on the fly; a restart is what makes
# the new file take effect.
if [ "$NO_RESTART" -eq 1 ]; then
    warn "config written but NOT applied: restart categraf yourself (systemctl restart categraf)"
    exit 0
fi
if command -v systemctl >/dev/null 2>&1 && systemctl cat categraf >/dev/null 2>&1; then
    systemctl restart categraf
    sleep 2
    systemctl is-active --quiet categraf \
        || die "categraf did not come back up; inspect with: journalctl -u categraf -n 50"
    log "categraf restarted; '$INPUT' metrics should appear in nightingale within a minute"
else
    # Non-systemd installs were started with nohup by the installer; guessing
    # the right process to kill from here is how unrelated agents get shot.
    warn "no systemd unit found; restart categraf manually to apply the new config"
fi
