#!/usr/bin/env bash
# GitHub official MCP toolsets / intentional inventory-change CI (#23 / #21).
# Lightweight stub (not full github-mcp-server). Hits real config_fingerprint
# and config_or_inventory_changed — not silent tools_list_drift.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [ -t 0 ] && [ -t 1 ]; then
  echo "note: this script is written for non-TTY CI; continuing without a prompt" >&2
fi

echo "== deadbugz-guard GitHub toolsets inventory change =="
go build -o bin/deadbugz-guard ./cmd/deadbugz-guard
go build -o bin/mock-mcp-adopt ./cmd/mock-mcp-adopt

WORKDIR="$(mktemp -d "${TMPDIR:-/tmp}/deadbugz-github.XXXXXX")"
trap 'rm -rf "$WORKDIR"' EXIT
PIN="$WORKDIR/github.json"

list_msg() {
  set +e
  python3 "$ROOT/scripts/mcp_list.py" --timeout 25 -- "$@"
  set -e
}

expect_allow() {
  local label="$1"
  shift
  local out
  out="$(list_msg "$@")"
  python3 -c "import json,sys
m=json.loads(sys.argv[1])
if m.get('error'):
    raise SystemExit('FAIL  '+sys.argv[2]+': '+json.dumps(m['error']))
names=[t.get('name') for t in m.get('result',{}).get('tools',[])]
print('PASS ', sys.argv[2], 'tools=', names)" "$out" "$label"
}

expect_reason() {
  local want="$1"
  local label="$2"
  shift 2
  local out
  out="$(list_msg "$@" || true)"
  python3 -c "import json,sys
m=json.loads(sys.argv[1])
err=m.get('error') or {}
data=err.get('data') or {}
got=str(data.get('reason_code') or '')
if got != sys.argv[2]:
    raise SystemExit(f'FAIL  {sys.argv[3]} reason={got!r} want {sys.argv[2]!r} body={m}')
print('PASS ', sys.argv[3], 'reason=' + got)" "$out" "$want" "$label"
}

./bin/deadbugz-guard approve --name github --pin "$PIN" -- \
  ./bin/mock-mcp-adopt github --toolsets repos

python3 - "$PIN" <<'PY'
import json, sys
p = json.load(open(sys.argv[1]))
assert p.get("config_fingerprint"), "pin missing config_fingerprint"
assert p.get("server_name") == "github"
print("  pinned github fingerprint", p["config_fingerprint"][:16])
PY

expect_allow "same --toolsets repos" \
  ./bin/deadbugz-guard --name github --pin "$PIN" -- \
  ./bin/mock-mcp-adopt github --toolsets repos

expect_reason config_or_inventory_changed "argv --toolsets repos,issues" \
  ./bin/deadbugz-guard --name github --pin "$PIN" -- \
  ./bin/mock-mcp-adopt github --toolsets repos,issues

expect_reason config_or_inventory_changed "GITHUB_TOOLSETS=repos,issues" \
  env GITHUB_TOOLSETS=repos,issues ./bin/deadbugz-guard --name github --pin "$PIN" -- \
  ./bin/mock-mcp-adopt github --toolsets repos

expect_reason tools_list_drift "same toolsets + ADOPT_POISON" \
  env ADOPT_POISON=1 ./bin/deadbugz-guard --name github --pin "$PIN" -- \
  ./bin/mock-mcp-adopt github --toolsets repos

# Flag bootstrap must not auto-promote an intentional inventory change.
expect_reason config_or_inventory_changed "--approve does not re-pin toolsets" \
  ./bin/deadbugz-guard --approve --name github --pin "$PIN" -- \
  ./bin/mock-mcp-adopt github --toolsets repos,issues

echo "PASS  GitHub toolsets inventory change is config_or_inventory_changed"
echo "ok"
