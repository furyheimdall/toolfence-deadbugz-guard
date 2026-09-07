#!/usr/bin/env bash
# CI-friendly Filesystem + Fetch attach (#23). Non-TTY approve from #27.
# Official-shaped stub by default (no npx/uvx). Does not expand smoke G/H/I/J.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [ -t 0 ] && [ -t 1 ]; then
  echo "note: this script is written for non-TTY CI/IDE spawn; continuing without a prompt" >&2
fi

echo "== deadbugz-guard Filesystem/Fetch attach (non-TTY) =="
go build -o bin/deadbugz-guard ./cmd/deadbugz-guard
go build -o bin/mock-mcp-adopt ./cmd/mock-mcp-adopt

WORKDIR="$(mktemp -d "${TMPDIR:-/tmp}/deadbugz-attach.XXXXXX")"
trap 'rm -rf "$WORKDIR"' EXIT
ALLOWED="$WORKDIR/allowed"
mkdir -p -m 0700 "$ALLOWED" "$WORKDIR/pins"

pin_ok() {
  local pin="$1"
  [ -f "$pin" ] || { echo "FAIL  missing pin $pin" >&2; exit 1; }
  local mode
  mode="$(python3 -c "import os,stat; print(oct(os.stat('$pin').st_mode & 0o777))")"
  if [ "$mode" != "0o600" ]; then
    echo "FAIL  pin mode $mode want 0o600 ($pin)" >&2
    exit 1
  fi
  python3 - "$pin" <<'PY'
import json, sys
p = json.load(open(sys.argv[1]))
for key in ("aggregate", "config_fingerprint", "pin_id", "server_name"):
    if not p.get(key):
        raise SystemExit(f"FAIL  pin missing {key}")
print("  pin", sys.argv[1], "hash=" + p["aggregate"][:12], "fp=" + p["config_fingerprint"][:12])
PY
}

list_ok() {
  local label="$1"
  shift
  local out
  out="$(python3 "$ROOT/scripts/mcp_list.py" --timeout 25 -- "$@")"
  python3 -c "import json,sys
m=json.loads(sys.argv[1])
err=m.get('error')
if err:
    raise SystemExit('FAIL  '+sys.argv[2]+': '+json.dumps(err))
names=[t.get('name') for t in m.get('result',{}).get('tools',[])]
print('PASS ', sys.argv[2], 'tools=', names[:4], '...')" \
    "$out" "$label"
}

list_reason() {
  local want="$1"
  local label="$2"
  shift 2
  set +e
  local out
  out="$(python3 "$ROOT/scripts/mcp_list.py" --timeout 25 -- "$@" 2>/dev/null)"
  local rc=$?
  set -e
  python3 -c "import json,sys
m=json.loads(sys.argv[1])
err=m.get('error') or {}
data=err.get('data') or {}
got=data.get('reason_code') or ''
if got != sys.argv[2]:
    raise SystemExit(f'FAIL  {sys.argv[3]} reason={got!r} want {sys.argv[2]!r} err={err}')
print('PASS ', sys.argv[3], 'reason=' + got)" "$out" "$want" "$label"
  [ "$rc" -ne 0 ] || true
}

FS_PIN="$WORKDIR/pins/filesystem.json"
FETCH_PIN="$WORKDIR/pins/fetch.json"

# A. CLI approve (no prompt) then wrap — matches plugin-guide / examples/cursor.mcp.json
./bin/deadbugz-guard approve --name filesystem --pin "$FS_PIN" -- \
  ./bin/mock-mcp-adopt filesystem "$ALLOWED"
pin_ok "$FS_PIN"
list_ok "filesystem wrap after approve" \
  ./bin/deadbugz-guard --name filesystem --pin "$FS_PIN" -- \
  ./bin/mock-mcp-adopt filesystem "$ALLOWED"

./bin/deadbugz-guard approve --name fetch --pin "$FETCH_PIN" -- \
  ./bin/mock-mcp-adopt fetch
pin_ok "$FETCH_PIN"
list_ok "fetch wrap after approve" \
  ./bin/deadbugz-guard --name fetch --pin "$FETCH_PIN" -- \
  ./bin/mock-mcp-adopt fetch

# B. IDE-spawn realistic: missing pin is PIN_MISSING; DEADBUGZ_APPROVE=1 bootstraps
BOOT_PIN="$WORKDIR/pins/boot-filesystem.json"
list_reason PIN_MISSING "filesystem wrap without pin" \
  ./bin/deadbugz-guard --name filesystem --pin "$BOOT_PIN" -- \
  ./bin/mock-mcp-adopt filesystem "$ALLOWED"
DEADBUGZ_APPROVE=1 list_ok "filesystem DEADBUGZ_APPROVE=1 bootstrap" \
  ./bin/deadbugz-guard --name filesystem --pin "$BOOT_PIN" -- \
  ./bin/mock-mcp-adopt filesystem "$ALLOWED"
pin_ok "$BOOT_PIN"

echo "PASS  Filesystem + Fetch attach and pin (non-TTY)"
echo "ok"
