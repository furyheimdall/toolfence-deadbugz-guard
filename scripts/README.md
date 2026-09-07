# Scripts

## `smoke.sh`

Fixture loop against `mock-mcp-deadbugz` (benign allow / poison deny). Not a live host attach.

A–F stay the existing sidecar/mockmcp tests. **G/H** (#32) are headless E2E in this script: wrap `--approve-file` after a nonempty `FLIP_PATH` diff, plus HITL hook/stub deny and timeout. **I** (#33) is headless wrap `PIN_MISSING` / pin tamper (no silent re-pin; explicit approve still required). **J** (#34) is a mid-session `FLIP_PATH` poison under an already-pinned wrap: in-flight `tools/list` is hashed before forward; subsequent `tools/call` denied.

`scripts/smokej` is a CI helper for that live wrap session. It is not a product command.

```bash
./scripts/smoke.sh
```

`scripts/smokehitl` is a CI helper for the HITL hook pin/audit path. It is not a product command.

## `verify-filesystem-fetch.sh` (E3 / #23)

Headless wrap + non-TTY pin of the Cursor Filesystem and Fetch launchers. No TTY.

```bash
# local (skips a launcher that is missing; mock fallback still proves wrap+pin)
./scripts/verify-filesystem-fetch.sh

# CI: npx + uvx must attach
./scripts/verify-filesystem-fetch.sh --ci
```

| Launcher | After `--` |
| --- | --- |
| Filesystem | `npx -y @modelcontextprotocol/server-filesystem <allowed-dir>` |
| Fetch | `uvx mcp-server-fetch` |

Non-TTY paths (from #20/#27): `deadbugz-guard approve`, wrap `--approve`, wrap `--approve-file`. Pin must be mode `0600`. A second `tools/list` allows when the catalog is unchanged.

`scripts/mcpprobe` is a CI helper (initialize + `tools/list`). It is not a product command.

### Skip gates

| Env / flag | Effect |
| --- | --- |
| `--skip-live` / `VERIFY_SKIP_LIVE=1` | Do not call npx/uvx; named mock wrap+pin only |
| `VERIFY_SKIP_FILESYSTEM=1` | Skip live Filesystem |
| `VERIFY_SKIP_FETCH=1` | Skip live Fetch |
| `--require-live` / `VERIFY_REQUIRE_LIVE=1` | Fail if a live launcher did not attach |

When a live launcher is missing, the script still runs wrap + non-TTY pin against `mock-mcp-deadbugz` (names `filesystem-mock` / `fetch-mock`) so offline/local verify is not empty. That fallback is **not** a Filesystem or Fetch attach — the skip line says so.

### E1 seam

This script stops at Filesystem + Fetch. GitHub official MCP toolsets / `config_or_inventory_changed` (#21) is a **separate** job — see `verify-github-toolsets` in `.github/workflows/ci.yml`. Do not add toolsets coverage here.

## `verify-github-toolsets.sh` (E1 / #23)

Intentional GitHub MCP inventory change (`--toolsets` / `--tools` / `GITHUB_TOOLSETS`) must surface `config_or_inventory_changed`, not silent `tools_list_drift`. Uses `mock-mcp-deadbugz` under the official attach shape. No `GITHUB_PERSONAL_ACCESS_TOKEN`.

```bash
./scripts/verify-github-toolsets.sh
```

Live `github-mcp-server` is a documented SKIP unless `DEADBUGZ_LIVE_GITHUB_MCP=1` plus a token / `DEADBUGZ_GITHUB_MCP_BIN`. Details: [docs/github-toolsets.md](../docs/github-toolsets.md).
