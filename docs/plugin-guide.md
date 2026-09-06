# Plugin guide

Attach **Deadbugz guard** as a stdio wrap in front of an MCP server the host already runs.

```
deadbugz-guard -- <server>
```

This is a sidecar on the existing MCP path. It is not a multi-server gateway.

## 1. Install the binary

Follow [README Install](../README.md#install): clone, `go build` `deadbugz-guard` (and `mock-mcp-deadbugz` if you want the smoke target), write a pin, put `deadbugz-guard` on `PATH`.

```bash
go build -o bin/deadbugz-guard ./cmd/deadbugz-guard
./bin/deadbugz-guard --write-pin --pin testdata/pin.json --from-mode benign
```

## 2. Host config shape

Cursor and Claude Desktop both use an `mcpServers` map. The shipped template is [`examples/plugin.json`](../examples/plugin.json).

Copy that object, then:

1. Point `command` at `deadbugz-guard` (absolute path if it is not on `PATH`).
2. Keep flags, then `--`, then **your** MCP server argv (`your-mcp-server` in the example).
3. Replace `/var/lib/deadbugz/pin.json` and `audit.jsonl` with writable local paths.
4. Point `--hitl` / `HITL_ENDPOINT` at the local HITL stub (`http://127.0.0.1:8765` in the README).

Host apps do **not** expand `${VAR}` inside `args`. Use real paths. Env keys (`PIN_PATH`, `AUDIT_PATH`, `HITL_ENDPOINT`, `CALL_GATE`) are optional duplicates — the binary reads them when flags are empty.

### Cursor

Merge `mcpServers` into the project file `.cursor/mcp.json`, or the user file `~/.cursor/mcp.json`. Restart Cursor (or reload MCP) so the wrap is the command the host launches.

### Claude Desktop

Merge the same `mcpServers` object into:

| OS | Config file |
| --- | --- |
| macOS | `~/Library/Application Support/Claude/claude_desktop_config.json` |
| Linux | `~/.config/Claude/claude_desktop_config.json` |
| Windows | `%APPDATA%\Claude\claude_desktop_config.json` |

Restart Claude Desktop after saving.

## 3. Smoke

Prove allow vs deny before wrapping a production server:

```bash
./scripts/smoke.sh
```

| Case | Result |
| --- | --- |
| **benign** (pinned) | **allow** |
| **poison** | **deny** (fail-closed) |

Details and the visual: [demo](demo.md). Compose (beside) stays in the [README](../README.md#compose-beside).
