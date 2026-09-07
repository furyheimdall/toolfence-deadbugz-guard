# Plugin guide

Attach **Deadbugz guard** as a stdio wrap in front of an MCP server the host already runs.

```
deadbugz-guard -- <server>
```

This is a sidecar on the existing MCP path. It is not a multi-server gateway.

## Cursor 60-second CTA

Wrap the Filesystem and Fetch servers Cursor already launches. Same pin → `tools/list` diff → fail-closed re-approval loop; the host keeps its MCP path.

### 1. Build, install, write a pin

Follow [README Install](../README.md#install): clone, `go build` `deadbugz-guard`, write a pin, put the binary on `PATH` (or use an absolute path in `command` below).

```bash
go build -o bin/deadbugz-guard ./cmd/deadbugz-guard
./bin/deadbugz-guard --write-pin --pin testdata/pin.json --from-mode benign
```

That `--write-pin` command is the smoke fixture (`mock-mcp-deadbugz` / `--from-mode benign`). It is not a Filesystem or Fetch pin. For a wrapped host server, point `--pin` at a **writable local file per server**. A `tools/list` mismatch fails closed until HITL re-approval.

### 2. Wrap Filesystem + Fetch in `mcp.json`

Merge into the project file `.cursor/mcp.json` or the user file `~/.cursor/mcp.json`. Set `command` to `deadbugz-guard`. Put the real server argv after `--`. Host apps do **not** expand `${VAR}` — use real absolute paths.

Copy-paste template: [`examples/cursor-mcp.filesystem-fetch.json`](../examples/cursor-mcp.filesystem-fetch.json). Shape matches [`examples/plugin.json`](../examples/plugin.json).

**Filesystem** — typical Cursor launch is `npx -y @modelcontextprotocol/server-filesystem` plus one or more allowed directories:

```json
{
  "mcpServers": {
    "filesystem": {
      "command": "deadbugz-guard",
      "args": [
        "--pin", "/absolute/path/to/deadbugz/pin-filesystem.json",
        "--audit", "/absolute/path/to/deadbugz/audit.jsonl",
        "--hitl", "http://127.0.0.1:8765",
        "--call-gate", "3",
        "--",
        "npx",
        "-y",
        "@modelcontextprotocol/server-filesystem",
        "/absolute/path/to/allowed/directory"
      ]
    }
  }
}
```

**Fetch** — many Cursor configs still use `npx -y @modelcontextprotocol/server-fetch`. The official reference server is often launched as `uvx mcp-server-fetch`. Wrap whichever argv you already run:

```json
{
  "mcpServers": {
    "fetch": {
      "command": "deadbugz-guard",
      "args": [
        "--pin", "/absolute/path/to/deadbugz/pin-fetch.json",
        "--audit", "/absolute/path/to/deadbugz/audit.jsonl",
        "--hitl", "http://127.0.0.1:8765",
        "--call-gate", "3",
        "--",
        "npx",
        "-y",
        "@modelcontextprotocol/server-fetch"
      ]
    }
  }
}
```

Alternate Fetch argv after `--`: `uvx`, `mcp-server-fetch`.

Use a **separate pin file per server**. Filesystem and Fetch advertise different `tools/list` menus.

If `deadbugz-guard` is not on `PATH`, set `command` to the absolute path of `bin/deadbugz-guard`. Optional `env` keys (`PIN_PATH`, `AUDIT_PATH`, `HITL_ENDPOINT`, `CALL_GATE`) match [`examples/plugin.json`](../examples/plugin.json).

### 3. Reload

Restart Cursor (or reload MCP) so the wrap is the command the host launches.

## FAQ

### Marketplace one-click ≠ guard wrap

Installing an MCP from the Cursor Marketplace does **not** put Deadbugz guard in front of that server. Marketplace install writes the vendor `command` / `args` (usually `npx …` or `uvx …`). You must set `command` to `deadbugz-guard` and put the real server argv after `--`. One-click enable is not a wrap.

### Cursor Auto-review ≠ Deadbugz pin approval

Cursor **Auto-review** is Cursor’s own action safety check. Deadbugz pin / re-approval is a **separate durable pin** on `tools/list`. Approving an Auto-review card does **not** re-pin Deadbugz. A mutated listing still fails closed until HITL re-approval on the guard’s pin.

## 1. Install the binary

Follow [README Install](../README.md#install): clone, `go build` `deadbugz-guard` (and `mock-mcp-deadbugz` if you want the smoke target), write a pin, put `deadbugz-guard` on `PATH`.

```bash
go build -o bin/deadbugz-guard ./cmd/deadbugz-guard
./bin/deadbugz-guard --write-pin --pin testdata/pin.json --from-mode benign
```

## 2. Host config shape

Cursor and Claude Desktop both use an `mcpServers` map. The shipped template is [`examples/plugin.json`](../examples/plugin.json). Cursor Filesystem + Fetch wrap: [60-second CTA](#cursor-60-second-cta) and [`examples/cursor-mcp.filesystem-fetch.json`](../examples/cursor-mcp.filesystem-fetch.json).

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
