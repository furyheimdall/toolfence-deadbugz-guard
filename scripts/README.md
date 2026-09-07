# Scripts

## `smoke.sh`

Fixture loop against `mock-mcp-deadbugz` (benign allow / poison deny). Not a live host attach.

```bash
./scripts/smoke.sh
```

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

This script stops at Filesystem + Fetch. GitHub official MCP toolsets / `config_or_inventory_changed` (#21) is a **separate** job — see the comment hook in `.github/workflows/ci.yml`. Do not add toolsets coverage here.
