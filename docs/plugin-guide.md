# Plugin guide

Attach **Deadbugz guard** as a stdio wrap in front of an MCP server the host already runs.

```
deadbugz-guard -- <server>
```

This is a sidecar on the existing MCP path. It is not a multi-server gateway.

## Cursor 60-second CTA

Wrap the Filesystem and Fetch servers Cursor already launches. Same pin → `tools/list` diff → fail-closed re-approval loop; the host keeps its MCP path.

`deadbugz-guard` is the process entry: `--pin`, then optional `--audit` / `--hitl` / `--call-gate`, then `--`, then the upstream tokens.

### 1. Build, install, write a pin

Follow [README Install](../README.md#install): clone, `go build` `deadbugz-guard`, write a pin, put the binary on `PATH` (or use an absolute path in `command` below).

```bash
go build -o bin/deadbugz-guard ./cmd/deadbugz-guard
./bin/deadbugz-guard --write-pin --pin testdata/pin.json --from-mode benign
```

That `--write-pin` command is the smoke fixture (`mock-mcp-deadbugz` / `--from-mode benign`). It is not a Filesystem or Fetch pin.

Suggested pin files (one per server): `~/.deadbugz/pins/filesystem.json` and `~/.deadbugz/pins/fetch.json`. Cursor does **not** expand `~` or `${VAR}` in `args` — write the absolute path (`/home/you/.deadbugz/pins/filesystem.json` on Linux, `/Users/you/.deadbugz/pins/filesystem.json` on macOS). A `tools/list` mismatch fails closed until HITL re-approval.

### 2. Wrap Filesystem + Fetch in `mcp.json`

Same `mcpServers` object in either file:

| Scope | File |
| --- | --- |
| Project | `.cursor/mcp.json` |
| Global | `~/.cursor/mcp.json` |

Set `command` to `deadbugz-guard`. After `--`, the upstream launchers are:

- **Filesystem:** `npx -y @modelcontextprotocol/server-filesystem <allowed-dir>`
- **Fetch:** `uvx mcp-server-fetch` (not the archived `@modelcontextprotocol/server-fetch` package)

Copy-paste template: [`examples/cursor-mcp.filesystem-fetch.json`](../examples/cursor-mcp.filesystem-fetch.json). Shape matches [`examples/plugin.json`](../examples/plugin.json).

**Filesystem**

```json
{
  "mcpServers": {
    "filesystem": {
      "command": "deadbugz-guard",
      "args": [
        "--pin", "/home/you/.deadbugz/pins/filesystem.json",
        "--audit", "/home/you/.deadbugz/audit.jsonl",
        "--hitl", "http://127.0.0.1:8765",
        "--call-gate", "3",
        "--",
        "npx",
        "-y",
        "@modelcontextprotocol/server-filesystem",
        "/home/you/allowed-dir"
      ]
    }
  }
}
```

**Fetch**

```json
{
  "mcpServers": {
    "fetch": {
      "command": "deadbugz-guard",
      "args": [
        "--pin", "/home/you/.deadbugz/pins/fetch.json",
        "--audit", "/home/you/.deadbugz/audit.jsonl",
        "--hitl", "http://127.0.0.1:8765",
        "--call-gate", "3",
        "--",
        "uvx",
        "mcp-server-fetch"
      ]
    }
  }
}
```

`--audit`, `--hitl`, and `--call-gate` are optional. If `deadbugz-guard` is not on `PATH`, set `command` to the absolute path of `bin/deadbugz-guard`. Optional `env` keys (`PIN_PATH`, `AUDIT_PATH`, `HITL_ENDPOINT`, `CALL_GATE`) match [`examples/plugin.json`](../examples/plugin.json).

### 3. Reload

Restart Cursor (or reload MCP) so the wrap is the command the host launches.

## FAQ

### Marketplace / one-click Add ≠ guard wrap

Marketplace / one-click **Add to Cursor** installs a **bare** server entry. It does **not** insert Deadbugz guard. After Marketplace install, change `command` to `deadbugz-guard` and put the original launcher after `--`.

### Cursor Auto-review ≠ Deadbugz pin / re-approval

Cursor **Auto-review** (and chat tool toggles) is not Deadbugz **pin / re-approval**. Approving an Auto-review card does not write or refresh a Deadbugz pin. Pin drift needs the Deadbugz HITL / re-approve path.

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
