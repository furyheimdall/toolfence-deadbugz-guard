#!/usr/bin/env bash
# FLIP_PATH smoke (atomic mv): benign allow, reorder allow, poison/add/remove deny,
# deadbugz + call_gate=3 block after gate.
# G/H (#32): headless re-approve + deny/timeout. I/J are other owners — do not add here.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "== deadbugz-guard smoke =="
go test ./internal/mockmcp ./sidecar -count=1 -timeout 60s -run 'TestSmoke|TestHash|TestDiff|TestWriteFlip|TestParseFlip'
echo "PASS  benign (pinned) → allow"
echo "PASS  reorder → allow"
echo "PASS  poison → deny"
echo "PASS  add / remove → fail-closed"
echo "PASS  deadbugz + call_gate=3 → block after gate"

# --- G/H E2E (#32): approve-file / HITL stub / env, no TTY ---
WORKDIR="$(mktemp -d -t deadbugz-gh-XXXXXX)"
GUARD="$WORKDIR/deadbugz-guard"
MOCK="$WORKDIR/mock-mcp-deadbugz"
PROBE="$WORKDIR/mcpprobe"
HITL="$WORKDIR/hitl"
SMOKEHITL="$WORKDIR/smokehitl"
HITL_PID=""

cleanup() {
  if [[ -n "${HITL_PID}" ]]; then
    kill "${HITL_PID}" 2>/dev/null || true
    wait "${HITL_PID}" 2>/dev/null || true
  fi
  rm -rf "$WORKDIR"
}
trap cleanup EXIT

echo "-- G/H build"
go build -o "$GUARD" ./cmd/deadbugz-guard
go build -o "$MOCK" ./cmd/mock-mcp-deadbugz
go build -o "$PROBE" ./scripts/mcpprobe
go build -o "$HITL" ./cmd/hitl
go build -o "$SMOKEHITL" ./scripts/smokehitl

write_flip() {
  local path="$1" mode="$2"
  local tmp
  tmp="$(mktemp "${path}.XXXXXX")"
  printf '{"mode":"%s","call_gate":3}\n' "$mode" >"$tmp"
  mv -f "$tmp" "$path"
}

pin_hash() {
  python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("aggregate",""))' "$1"
}

assert_audit_g() {
  python3 - "$1" "$2" "$3" <<'PY'
import json, sys
path, old, new = sys.argv[1], sys.argv[2], sys.argv[3]
rows = []
with open(path) as f:
    for line in f:
        line = line.strip()
        if line:
            rows.append(json.loads(line))
approved = [r for r in rows if r.get("event") == "approved"]
if not approved:
    sys.exit("missing approved event")
r = approved[-1]
for k in ("who", "when", "oldHash", "newHash"):
    if not r.get(k):
        sys.exit("missing %s: %s" % (k, r))
if r["oldHash"] != old:
    sys.exit("oldHash %s != %s" % (r["oldHash"], old))
if r["newHash"] != new:
    sys.exit("newHash %s != %s" % (r["newHash"], new))
if r["oldHash"] == r["newHash"]:
    sys.exit("oldHash == newHash after approve")
PY
}

wait_http() {
  python3 - "$1" <<'PY'
import sys, time, urllib.request
url = sys.argv[1]
for _ in range(80):
    try:
        urllib.request.urlopen(url, timeout=0.2)
        sys.exit(0)
    except Exception:
        time.sleep(0.05)
sys.exit("HITL stub did not become ready: " + url)
PY
}

# G: approve-file wrap after nonempty FLIP_PATH diff → pin advances + audit
GDIR="$WORKDIR/g"
mkdir -p "$GDIR"
GFLIP="$GDIR/flip.json"
GPIN="$GDIR/pin.json"
GAUDIT="$GDIR/audit.jsonl"
GTOK="$GDIR/approve"
write_flip "$GFLIP" benign
"$GUARD" --write-pin --pin "$GPIN" --from-mode benign
G_OLD="$(pin_hash "$GPIN")"
if [[ -z "$G_OLD" ]]; then
  echo "FAIL  G: empty old pin hash" >&2
  exit 1
fi
write_flip "$GFLIP" poison
printf 'approve\n' >"$GTOK"
chmod 600 "$GTOK"
FLIP_PATH="$GFLIP" "$PROBE" --expect-allow --timeout 20s -- \
  "$GUARD" --pin "$GPIN" --audit "$GAUDIT" --approve-file "$GTOK" --name smoke-g -- \
  "$MOCK"
G_NEW="$(pin_hash "$GPIN")"
if [[ -z "$G_NEW" || "$G_NEW" == "$G_OLD" ]]; then
  echo "FAIL  G: approve-file did not advance pin (old=$G_OLD new=$G_NEW)" >&2
  exit 1
fi
if [[ -e "$GTOK" ]]; then
  echo "FAIL  G: approve-file was not consumed" >&2
  exit 1
fi
assert_audit_g "$GAUDIT" "$G_OLD" "$G_NEW"

# G: HITL hook (smokehitl) also advances pin + audit who/when/oldHash/newHash
GHITL="$WORKDIR/g-hitl"
mkdir -p "$GHITL"
HPIN="$GHITL/pin.json"
HAUDIT="$GHITL/audit.jsonl"
"$GUARD" --write-pin --pin "$HPIN" --from-mode benign
H_OLD="$(pin_hash "$HPIN")"
"$SMOKEHITL" -pin "$HPIN" -audit "$HAUDIT" -decision approve -who smoke-g -live poison
H_NEW="$(pin_hash "$HPIN")"
if [[ -z "$H_NEW" || "$H_NEW" == "$H_OLD" ]]; then
  echo "FAIL  G: HITL approve did not advance pin" >&2
  exit 1
fi
assert_audit_g "$HAUDIT" "$H_OLD" "$H_NEW"
echo "PASS  G re-approve → pin newHash + audit who/when/oldHash/newHash"

# H deny: wrap without token after poison — old pin kept (no silent promote)
HDIR="$WORKDIR/h-deny"
mkdir -p "$HDIR"
HFLIP="$HDIR/flip.json"
HPIN2="$HDIR/pin.json"
write_flip "$HFLIP" benign
"$GUARD" --write-pin --pin "$HPIN2" --from-mode benign
H_OLD="$(pin_hash "$HPIN2")"
write_flip "$HFLIP" poison
FLIP_PATH="$HFLIP" "$PROBE" --expect-deny DIFF_NONEMPTY --timeout 20s -- \
  "$GUARD" --pin "$HPIN2" --name smoke-h -- \
  "$MOCK"
H_KEEP="$(pin_hash "$HPIN2")"
if [[ "$H_KEEP" != "$H_OLD" ]]; then
  echo "FAIL  H deny wrap silently promoted pin (old=$H_OLD new=$H_KEEP)" >&2
  exit 1
fi

# H deny: HITL hook ApplyApproval(deny) keeps old pin
HD="$WORKDIR/h-hitl-deny"
mkdir -p "$HD"
"$GUARD" --write-pin --pin "$HD/pin.json" --from-mode benign
HD_OLD="$(pin_hash "$HD/pin.json")"
"$SMOKEHITL" -pin "$HD/pin.json" -audit "$HD/audit.jsonl" -decision deny -who smoke-h -live poison
if [[ "$(pin_hash "$HD/pin.json")" != "$HD_OLD" ]]; then
  echo "FAIL  H HITL deny promoted pin" >&2
  exit 1
fi

# H timeout: HITL hook timeout == deny, old pin kept
HT="$WORKDIR/h-hitl-timeout"
mkdir -p "$HT"
"$GUARD" --write-pin --pin "$HT/pin.json" --from-mode benign
HT_OLD="$(pin_hash "$HT/pin.json")"
"$SMOKEHITL" -pin "$HT/pin.json" -audit "$HT/audit.jsonl" -decision timeout -who smoke-h -live poison -timeout 50ms
if [[ "$(pin_hash "$HT/pin.json")" != "$HT_OLD" ]]; then
  echo "FAIL  H HITL timeout promoted pin" >&2
  exit 1
fi

# H timeout: HITL HTTP stub (cmd/hitl) times out without writing/promoting the pin
HS="$WORKDIR/h-stub"
mkdir -p "$HS"
"$GUARD" --write-pin --pin "$HS/pin.json" --from-mode benign
HS_OLD="$(pin_hash "$HS/pin.json")"
HITL_PORT=$((18000 + RANDOM % 2000))
"$HITL" serve -listen "127.0.0.1:${HITL_PORT}" >/dev/null 2>&1 &
HITL_PID=$!
wait_http "http://127.0.0.1:${HITL_PORT}/health"
if "$HITL" request -base "http://127.0.0.1:${HITL_PORT}" -timeout 1; then
  echo "FAIL  H HITL stub request should timeout/deny" >&2
  exit 1
fi
if [[ "$(pin_hash "$HS/pin.json")" != "$HS_OLD" ]]; then
  echo "FAIL  H HITL stub timeout promoted pin" >&2
  exit 1
fi
echo "PASS  H deny/timeout → old pin kept"

echo "ok"
