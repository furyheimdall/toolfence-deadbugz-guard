#!/usr/bin/env bash
# FLIP_PATH smoke (atomic mv): benign allow, reorder allow, poison/add/remove deny,
# deadbugz + call_gate=3 block after gate.
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
echo "ok"
