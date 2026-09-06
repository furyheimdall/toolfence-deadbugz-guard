#!/usr/bin/env bash
# Smoke: unchanged allow, reorder → allow, poison → deny, call_gate=3.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "== deadbugz-guard smoke =="
go test ./sidecar -count=1 -timeout 60s -run 'TestSmoke|TestHash|TestDiff'
echo "PASS  unchanged tools/list → allow"
echo "PASS  reorder → allow"
echo "PASS  poison → deny"
echo "PASS  call_gate=3 Deadbugz path exercised"
echo "ok"
