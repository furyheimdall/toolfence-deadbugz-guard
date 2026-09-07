#!/usr/bin/env bash
# E3 (#23): scripted, CI-friendly verify that Deadbugz wrap of Filesystem + Fetch
# attaches and pins cleanly using non-TTY approve from #20/#27.
#
# Headless only — no TTY prompt. Paths used:
#   deadbugz-guard approve -- <launcher>
#   deadbugz-guard --approve -- <launcher>
#   deadbugz-guard --approve-file TOKEN -- <launcher>
#
# This script is Filesystem + Fetch only.
# E1 seam (#23): GitHub official MCP toolsets / config_or_inventory_changed
# belongs in a separate script/job. Do not extend this file into toolsets CI.
#
# Usage:
#   ./scripts/verify-filesystem-fetch.sh [--ci] [--require-live] [--skip-live]
#
# Env:
#   VERIFY_REQUIRE_LIVE=1  fail if a live launcher cannot attach (CI default with --ci)
#   VERIFY_SKIP_LIVE=1     skip npx/uvx; still run named mock wrap+pin (local/offline)
#   VERIFY_SKIP_FILESYSTEM=1 / VERIFY_SKIP_FETCH=1
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

REQUIRE_LIVE="${VERIFY_REQUIRE_LIVE:-0}"
SKIP_LIVE="${VERIFY_SKIP_LIVE:-0}"
CI_MODE=0
SKIP_FS="${VERIFY_SKIP_FILESYSTEM:-0}"
SKIP_FETCH="${VERIFY_SKIP_FETCH:-0}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --ci) CI_MODE=1; REQUIRE_LIVE=1; shift ;;
    --require-live) REQUIRE_LIVE=1; shift ;;
    --skip-live) SKIP_LIVE=1; REQUIRE_LIVE=0; shift ;;
    -h|--help)
      sed -n '2,20p' "$0"
      exit 0
      ;;
    *)
      echo "unknown arg: $1" >&2
      exit 2
      ;;
  esac
done

WORKDIR="$(mktemp -d -t deadbugz-e3-XXXXXX)"
GUARD="$WORKDIR/deadbugz-guard"
PROBE="$WORKDIR/mcpprobe"
MOCK="$WORKDIR/mock-mcp-deadbugz"
PASS=0
FAIL=0
SKIP=0
SKIPPED=()

cleanup() {
  rm -rf "$WORKDIR"
}
trap cleanup EXIT

log() { printf '%s\n' "$*"; }
ok() { PASS=$((PASS + 1)); log "PASS  $*"; }
fail() { FAIL=$((FAIL + 1)); log "FAIL  $*"; }
skip() { SKIP=$((SKIP + 1)); SKIPPED+=("$*"); log "SKIP  $*"; }

pin_mode() {
  local path="$1"
  if stat -c '%a' "$path" >/dev/null 2>&1; then
    stat -c '%a' "$path"
  else
    stat -f '%OLp' "$path"
  fi
}

assert_pin_0600() {
  local path="$1"
  if [[ ! -f "$path" ]]; then
    fail "pin missing: $path"
    return 1
  fi
  local mode
  mode="$(pin_mode "$path")"
  if [[ "$mode" != "600" ]]; then
    fail "pin mode=$mode want 0600 ($path)"
    return 1
  fi
  return 0
}

pin_tool_names() {
  python3 -c 'import json,sys; p=json.load(open(sys.argv[1])); print(" ".join(t.get("name","") for t in p.get("tools",[])))' "$1"
}

pin_has_any_tool() {
  local path="$1"
  shift
  python3 - "$path" "$@" <<'PY'
import json, sys
path, want = sys.argv[1], sys.argv[2:]
p = json.load(open(path))
names = {t.get("name", "") for t in p.get("tools", [])}
sys.exit(0 if names.intersection(want) else 1)
PY
}

have_cmd() { command -v "$1" >/dev/null 2>&1; }

# Pre-warm so approve's 45s timeout is not spent on first package download.
# Official Filesystem/Fetch exit when stdin is already closed; timeout is a
# safety net if a launcher blocks (coreutils timeout; optional on macOS).
prewarm() {
  local label="$1"
  shift
  log "-- prewarm $label: $*"
  if have_cmd timeout; then
    if timeout 90 "$@" </dev/null; then
      return 0
    fi
    local rc=$?
    # 124 = timeout (server started and blocked on stdin — cache is warm)
    if [[ $rc -eq 124 ]]; then
      return 0
    fi
    return $rc
  fi
  "$@" </dev/null
}

log "== deadbugz-guard Filesystem + Fetch verify (E3 / #23) =="
log "workdir $WORKDIR  ci=$CI_MODE require_live=$REQUIRE_LIVE skip_live=$SKIP_LIVE"

log "-- build"
go build -o "$GUARD" ./cmd/deadbugz-guard
go build -o "$PROBE" ./scripts/mcpprobe
go build -o "$MOCK" ./cmd/mock-mcp-deadbugz

# --- live launcher resolution ------------------------------------------------

FS_LAUNCHER=()
FETCH_LAUNCHER=()
ALLOWED_DIR="$WORKDIR/allowed"
mkdir -p -m 0700 "$ALLOWED_DIR" "$WORKDIR/pins" "$WORKDIR/approve"
printf 'ok\n' >"$ALLOWED_DIR/hello.txt"

if [[ "$SKIP_LIVE" != "1" && "$SKIP_FS" != "1" ]]; then
  if have_cmd npx; then
    FS_LAUNCHER=(npx -y @modelcontextprotocol/server-filesystem "$ALLOWED_DIR")
  else
    skip "filesystem live: npx not on PATH"
  fi
elif [[ "$SKIP_FS" == "1" ]]; then
  skip "filesystem live: VERIFY_SKIP_FILESYSTEM=1"
fi

if [[ "$SKIP_LIVE" != "1" && "$SKIP_FETCH" != "1" ]]; then
  if have_cmd uvx; then
    FETCH_LAUNCHER=(uvx mcp-server-fetch)
  else
    skip "fetch live: uvx not on PATH"
  fi
elif [[ "$SKIP_FETCH" == "1" ]]; then
  skip "fetch live: VERIFY_SKIP_FETCH=1"
fi

if [[ "$SKIP_LIVE" == "1" ]]; then
  skip "live launchers: --skip-live / VERIFY_SKIP_LIVE=1"
fi

# --- one server: three non-TTY approve paths + unchanged list allow ----------

verify_wrap() {
  local name="$1"
  local expect_tools_csv="$2"
  shift 2
  local pin="$WORKDIR/pins/${name}.json"
  local tok="$WORKDIR/approve/${name}"
  local IFS=','
  # shellcheck disable=SC2206
  local expect_tools=($expect_tools_csv)
  unset IFS

  log ""
  log "## $name  launcher: $*"

  rm -f "$pin" "${pin}.pending" "$tok"

  log "-- $name: approve CLI (no TTY)"
  if ! "$GUARD" approve --name "$name" --pin "$pin" -- "$@"; then
    fail "$name approve CLI"
    return 1
  fi
  if ! assert_pin_0600 "$pin"; then
    return 1
  fi
  if [[ ${#expect_tools[@]} -gt 0 ]] && ! pin_has_any_tool "$pin" "${expect_tools[@]}"; then
    fail "$name pin missing expected tools (${expect_tools[*]}); got: $(pin_tool_names "$pin")"
    return 1
  fi
  ok "$name approve CLI wrote 0600 pin tools=[$(pin_tool_names "$pin")]"

  log "-- $name: wrap tools/list allows when unchanged"
  if ! "$PROBE" --expect-allow --timeout 60s -- \
    "$GUARD" --name "$name" --pin "$pin" -- "$@"; then
    fail "$name wrap list after approve"
    return 1
  fi
  ok "$name subsequent list allows (unchanged pin)"

  log "-- $name: wrap --approve first pin (no TTY)"
  rm -f "$pin" "${pin}.pending"
  if ! "$PROBE" --expect-allow --timeout 60s -- \
    "$GUARD" --name "$name" --pin "$pin" --approve -- "$@"; then
    fail "$name wrap --approve"
    return 1
  fi
  if ! assert_pin_0600 "$pin"; then
    return 1
  fi
  ok "$name --approve wrote 0600 pin"

  log "-- $name: wrap --approve-file first pin (no TTY)"
  rm -f "$pin" "${pin}.pending"
  printf 'approve\n' >"$tok"
  chmod 600 "$tok"
  if ! "$PROBE" --expect-allow --timeout 60s -- \
    "$GUARD" --name "$name" --pin "$pin" --approve-file "$tok" -- "$@"; then
    fail "$name wrap --approve-file"
    return 1
  fi
  if ! assert_pin_0600 "$pin"; then
    return 1
  fi
  if [[ -e "$tok" ]]; then
    fail "$name approve-file was not consumed"
    return 1
  fi
  ok "$name --approve-file wrote 0600 pin and consumed token"
}

# --- Filesystem (npx) --------------------------------------------------------

if [[ ${#FS_LAUNCHER[@]} -gt 0 ]]; then
  if prewarm filesystem "${FS_LAUNCHER[@]}"; then
    verify_wrap filesystem "read_file,read_text_file,list_directory" "${FS_LAUNCHER[@]}" || true
  else
    skip "filesystem live: prewarm/attach failed (${FS_LAUNCHER[*]})"
    FS_LAUNCHER=()
  fi
fi

# --- Fetch (uvx) -------------------------------------------------------------

if [[ ${#FETCH_LAUNCHER[@]} -gt 0 ]]; then
  if prewarm fetch "${FETCH_LAUNCHER[@]}"; then
    verify_wrap fetch "fetch" "${FETCH_LAUNCHER[@]}" || true
  else
    skip "fetch live: prewarm/attach failed (${FETCH_LAUNCHER[*]})"
    FETCH_LAUNCHER=()
  fi
fi

# --- mock fallback (named wrap+pin only; not a live Filesystem/Fetch attach) -

need_mock=0
if [[ ${#FS_LAUNCHER[@]} -eq 0 && "$SKIP_FS" != "1" ]]; then
  need_mock=1
fi
if [[ ${#FETCH_LAUNCHER[@]} -eq 0 && "$SKIP_FETCH" != "1" ]]; then
  need_mock=1
fi

if [[ "$need_mock" == "1" ]]; then
  log ""
  log "## mock fallback (wrap + non-TTY pin; not a live Filesystem/Fetch attach)"
  if [[ ${#FS_LAUNCHER[@]} -eq 0 && "$SKIP_FS" != "1" ]]; then
    verify_wrap filesystem-mock "" "$MOCK" || true
  fi
  if [[ ${#FETCH_LAUNCHER[@]} -eq 0 && "$SKIP_FETCH" != "1" ]]; then
    verify_wrap fetch-mock "" "$MOCK" || true
  fi
fi

# --- summary -----------------------------------------------------------------

log ""
log "== summary  pass=$PASS  fail=$FAIL  skip=$SKIP =="
if [[ ${#SKIPPED[@]} -gt 0 ]]; then
  log "skip gates:"
  for s in "${SKIPPED[@]}"; do
    log "  - $s"
  done
fi

if [[ "$FAIL" -gt 0 ]]; then
  log "FAILED"
  exit 1
fi

if [[ "$REQUIRE_LIVE" == "1" ]]; then
  if [[ ${#FS_LAUNCHER[@]} -eq 0 && "$SKIP_FS" != "1" ]]; then
    log "FAIL  require-live: Filesystem npx launcher did not attach"
    exit 1
  fi
  if [[ ${#FETCH_LAUNCHER[@]} -eq 0 && "$SKIP_FETCH" != "1" ]]; then
    log "FAIL  require-live: Fetch uvx launcher did not attach"
    exit 1
  fi
fi

# E1 seam (#23): stop here. Next job/script should wrap GitHub official MCP
# toolsets and assert config_or_inventory_changed on intentional inventory
# change. Do not add that coverage in this file.
log "ok  (E1 hook: add toolsets / config_or_inventory_changed in a separate job)"
exit 0
