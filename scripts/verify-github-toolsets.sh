#!/usr/bin/env bash
# GitHub official MCP toolsets / intentional inventory-change verify (#23 / #21).
#
# Always-on path (CI, no secrets):
#   Wrap + CLI fixtures use mock-mcp-deadbugz under the official github-mcp-server
#   attach shape (--toolsets / --tools / GITHUB_TOOLSETS). Prove Evaluate surfaces
#   config_or_inventory_changed (expected re-approval), not silent tools_list_drift.
#
# Live official server (opt-in only):
#   github-mcp-server requires GITHUB_PERSONAL_ACCESS_TOKEN and a network install.
#   CI never enables this. To try locally:
#     DEADBUGZ_LIVE_GITHUB_MCP=1 GITHUB_PERSONAL_ACCESS_TOKEN=… ./scripts/verify-github-toolsets.sh
#   Missing binary or token is a documented SKIP, not a failure.
#
# Does not expand smoke G/H/I/J. Filesystem + Fetch wrap verify is a separate #23 slice.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "== GitHub MCP toolsets / config_or_inventory_changed verify =="
echo "fixture: mock-mcp-deadbugz as github-mcp-server (no GitHub credentials)"

go test ./core -count=1 -timeout 60s -run 'TestEvaluateConfigChange|TestConfigFingerprintEnvInventoryChange|TestApplyApprovalConfigChange|TestEvaluateToolsListDrift'
go test ./sidecar -count=1 -timeout 120s -run 'GitHubToolsets|GitHubSameConfig|GitHubApproveFile|GitHubToolsFlag|GitHubSameFingerprint'

if [[ "${DEADBUGZ_LIVE_GITHUB_MCP:-}" == "1" ]]; then
  if [[ -z "${GITHUB_PERSONAL_ACCESS_TOKEN:-}" ]]; then
    echo "SKIP live github-mcp-server: GITHUB_PERSONAL_ACCESS_TOKEN unset (required by the official server)"
  elif ! command -v github-mcp-server >/dev/null 2>&1 && [[ -z "${DEADBUGZ_GITHUB_MCP_BIN:-}" ]]; then
    echo "SKIP live github-mcp-server: binary not on PATH (set DEADBUGZ_GITHUB_MCP_BIN or install github/github-mcp-server)"
  else
    echo "SKIP live github-mcp-server: opt-in live listing is not wired (secrets/network). Fixture path above is the CI contract."
  fi
else
  echo "SKIP live github-mcp-server (set DEADBUGZ_LIVE_GITHUB_MCP=1 plus GITHUB_PERSONAL_ACCESS_TOKEN to opt in)"
fi

echo "PASS  --toolsets / --tools / GITHUB_TOOLSETS change → config_or_inventory_changed"
echo "PASS  same fingerprint + live tools/list mutate → tools_list_drift"
echo "ok"
