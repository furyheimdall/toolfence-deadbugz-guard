# Plugin guide

Attach **Deadbugz guard** as a stdio wrap in front of an MCP server the host already runs.

```
deadbugz-guard -- <server> [args...]
```

This is a sidecar on the existing MCP path. It is not a multi-server gateway.

Cursor is the primary attach. Merge `mcpServers` into the **project** file `.cursor/mcp.json` or the **user** file `~/.cursor/mcp.json`. Ready-to-copy shapes: [`examples/plugin.json`](../examples/plugin.json), [`examples/cursor.mcp.json`](../examples/cursor.mcp.json), and [`examples/cursor-mcp.filesystem-fetch.json`](../examples/cursor-mcp.filesystem-fetch.json).

## Cursor 60-second CTA

Wrap the Filesystem and Fetch servers Cursor already launches. Same pin → `tools/list` diff → fail-closed re-approval loop; the host keeps its MCP path.

`deadbugz-guard` is the process entry: `--pin`, then optional `--audit` / `--hitl` / `--call-gate`, then `--`, then the upstream tokens.

### 1. Build, install, write a pin

Follow [README Install](../README.md#install): clone, `go build` `deadbugz-guard`, write a pin, put the binary on `PATH` (or use an absolute path in `command` below).

```bash
go build -o bin/deadbugz-guard ./cmd/deadbugz-guard
./bin/deadbugz-guard --write-pin --pin testdata/pin.json --from-mode benign
```

That `--write-pin` command is the smoke fixture (`mock-mcp-deadbugz` / `--from-mode benign`). It is not a Filesystem or Fetch pin. For a live server with no TTY, use `deadbugz-guard approve` ([non-TTY bootstrap](#3-non-tty-bootstrap--approve)).

Suggested pin files (one per server): `~/.deadbugz/pins/filesystem.json` and `~/.deadbugz/pins/fetch.json`. Cursor does **not** expand `~` or `${VAR}` in `args` — write the absolute path (`/home/you/.deadbugz/pins/filesystem.json` on Linux, `/Users/you/.deadbugz/pins/filesystem.json` on macOS), or use `~/…` and let the binary expand it. A `tools/list` mismatch fails closed until HITL / non-TTY re-approval.

### 2. Wrap Filesystem + Fetch in `mcp.json`

Same `mcpServers` object in either file:

| Scope | File |
| --- | --- |
| Project | `.cursor/mcp.json` |
| Global | `~/.cursor/mcp.json` |

Set `command` to `deadbugz-guard`. After `--`, the upstream launchers are:

- **Filesystem:** `npx -y @modelcontextprotocol/server-filesystem <allowed-dir>`
- **Fetch:** `uvx mcp-server-fetch` (not the archived `@modelcontextprotocol/server-fetch` package)

Copy-paste template: [`examples/cursor-mcp.filesystem-fetch.json`](../examples/cursor-mcp.filesystem-fetch.json). Shape matches [`examples/plugin.json`](../examples/plugin.json) and [`examples/cursor.mcp.json`](../examples/cursor.mcp.json).

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

Cursor **Auto-review** (and chat tool toggles) is not Deadbugz **pin / re-approval**. Approving an Auto-review card does not write or refresh a Deadbugz pin. Pin drift needs the Deadbugz HITL / non-TTY re-approve path.

## 1. Install the binary

Follow [README Install](../README.md#install): clone, `go build` `deadbugz-guard`.

```bash
go build -o bin/deadbugz-guard ./cmd/deadbugz-guard
```

**PATH / absolute path.** Cursor (and other IDEs) often spawn MCP with a minimal `PATH` and do **not** load `~/.bashrc` / `~/.zshrc`. Prefer an **absolute path** to `deadbugz-guard` in `command`. If you wrap `npx` or `uvx`, give those an absolute path too, or set `PATH` in the server `env` block.

```json
"command": "/ABS/bin/deadbugz-guard"
```

## 2. Pin path (per server, mode 0600)

One pin file per MCP server name. Suggested layout (survey):

| Server | Pin file |
| --- | --- |
| Filesystem | `~/.deadbugz/pins/filesystem.json` |
| Fetch | `~/.deadbugz/pins/fetch.json` |

`--name filesystem` (or `DEADBUGZ_SERVER_NAME=filesystem`) fills `--pin` with that default when `--pin` / `PIN_PATH` is empty.

The binary expands a leading `~/` so you can write `~/.deadbugz/pins/filesystem.json` even when the host does not expand tildes.

Create the directory once. Pin files are written **mode 0600** (owner read/write only). Keep the directory `0700`.

```bash
mkdir -p -m 0700 ~/.deadbugz/pins ~/.deadbugz/approve
```

Host apps do **not** expand `${VAR}` inside `args`. Use real paths, `~/…`, or `--name`.

## 3. Non-TTY bootstrap / approve

Cursor has **no** pin-drift dialog and **no** TTY on the spawned wrap. The wrap never prompts.

**First `tools/list`:** if the pin file is missing, the listing is **fail-closed** (`PIN_MISSING`) unless you grant a headless approve. A missing or **tampered** pin is never silently repaired.

**After you approve:** the pin is durable. Later drift (`DIFF_NONEMPTY` / `tools_list_drift`) fails closed. There is no silent re-pin.

### Headless approve paths (pick one)

**A. CLI (works in CI / a script / a real terminal — no interactive prompt)**

```bash
# Filesystem: allowed-dir args stay AFTER --
deadbugz-guard approve --name filesystem --pin ~/.deadbugz/pins/filesystem.json -- \
  /ABS/npx -y @modelcontextprotocol/server-filesystem /ABS/allowed-dir

# Fetch via uvx
deadbugz-guard approve --name fetch --pin ~/.deadbugz/pins/fetch.json -- \
  /ABS/uvx mcp-server-fetch
```

`--write-pin -- <server>` is the same live-list write (fixture `--from-mode` still works without `--`).

If the wrap already failed closed, it wrote `<pin>.pending`. Approve that snapshot without talking to the IDE process:

```bash
deadbugz-guard approve --pin ~/.deadbugz/pins/filesystem.json
```

**B. Flag / env on the wrap (IDE spawn, first pin only)**

Set `DEADBUGZ_APPROVE=1` or `--approve` in `mcp.json`. The first `tools/list` with a **missing** pin file writes `~/.deadbugz/pins/<name>.json` at 0600 and allows. Drift still fails closed. A tampered file is **not** overwritten.

**C. One-shot file (bootstrap or explicit drift re-pin)**

```bash
echo approve > ~/.deadbugz/approve/filesystem
```

Point `--approve-file` / `DEADBUGZ_APPROVE_FILE` at that path. After a successful write the file is deleted. This is the only wrap-side grant that may advance a pin on `DIFF_NONEMPTY` / `tools_list_drift`.

**D. HTTP GET (bootstrap only, no long-poll)**

`DEADBUGZ_APPROVE=http://127.0.0.1:8765/v1/bootstrap` — GET, 2s timeout, body `{"decision":"approve"}` or `approve`. Does not wait on the HITL long-poll stub.

Existing `hitl serve` / `hitl decide` stays the local HITL demo. It is not a TTY prompt; it does not write the wrap pin. Use `deadbugz-guard approve` to persist.

Reload MCP (or wait for the next `tools/list`) after an out-of-band approve. The wrap re-reads the pin file each listing.

## 4. Cursor attach snippets

Project: `.cursor/mcp.json`. Global: `~/.cursor/mcp.json`. Same `mcpServers` object.

Replace `/ABS` with real paths. Allowed-directory arguments for Filesystem stay **after** `--`.

### Filesystem (`@modelcontextprotocol/server-filesystem`)

```json
{
  "mcpServers": {
    "filesystem": {
      "command": "/ABS/bin/deadbugz-guard",
      "args": [
        "--name", "filesystem",
        "--pin", "~/.deadbugz/pins/filesystem.json",
        "--audit", "~/.deadbugz/audit/filesystem.jsonl",
        "--",
        "/ABS/npx",
        "-y",
        "@modelcontextprotocol/server-filesystem",
        "/ABS/allowed-dir"
      ],
      "env": {
        "DEADBUGZ_SERVER_NAME": "filesystem",
        "PIN_PATH": "/HOME/.deadbugz/pins/filesystem.json"
      }
    }
  }
}
```

### Fetch (`uvx mcp-server-fetch`)

```json
{
  "mcpServers": {
    "fetch": {
      "command": "/ABS/bin/deadbugz-guard",
      "args": [
        "--name", "fetch",
        "--pin", "~/.deadbugz/pins/fetch.json",
        "--audit", "~/.deadbugz/audit/fetch.jsonl",
        "--",
        "/ABS/uvx",
        "mcp-server-fetch"
      ],
      "env": {
        "DEADBUGZ_SERVER_NAME": "fetch",
        "PIN_PATH": "/HOME/.deadbugz/pins/fetch.json"
      }
    }
  }
}
```

Optional first-run bootstrap in `env` (missing pin only): `"DEADBUGZ_APPROVE": "1"`. Remove it after the pin exists if you do not want a leftover grant on a deleted pin file.

Restart Cursor or reload MCP so the wrap is the command the host launches.

## 5. Drift / `PIN_MISSING` in Cursor

Fail-closed results are **JSON-RPC errors** on `tools/list` and (with `call_gate=3`) `tools/call`. Cursor shows them as a **tool-call failure**. The error `message` is `deadbugz-guard: PIN_MISSING` or `deadbugz-guard: DIFF_NONEMPTY`. `data.reason_code`, `data.pin_path`, `data.diff`, and `data.approve_hint` travel with the error.

**MCP Logs:** Cursor → **Output** panel → **MCP Logs** (or the MCP server log channel). The wrap writes the same reason, pin path, and approve hint to **stderr**. There is no in-IDE pin-drift dialog.

Then run a non-TTY approve (section 3) and retry the tool.

## 6. Host config notes

Flags / env: `--pin` / `PIN_PATH`, `--name` / `DEADBUGZ_SERVER_NAME`, `--approve` / `DEADBUGZ_APPROVE`, `--approve-file` / `DEADBUGZ_APPROVE_FILE`, `--audit` / `AUDIT_PATH`, `--hitl` / `HITL_ENDPOINT`, `--call-gate` / `CALL_GATE` (default **3**).

Env keys are optional duplicates — the binary reads them when flags are empty.

### Claude Desktop

Merge the same `mcpServers` object into:

| OS | Config file |
| --- | --- |
| macOS | `~/Library/Application Support/Claude/claude_desktop_config.json` |
| Linux | `~/.config/Claude/claude_desktop_config.json` |
| Windows | `%APPDATA%\Claude\claude_desktop_config.json` |

Restart Claude Desktop after saving.

Marketplace auto-write of `mcp.json` is **out** (CTA / docs only).

## 7. Smoke

Prove allow vs deny before wrapping a production server:

```bash
./scripts/smoke.sh
```

| Case | Result |
| --- | --- |
| **benign** (pinned) | **allow** |
| **poison** | **deny** (fail-closed) |

Details and the visual: [demo](demo.md). Compose (beside) stays in the [README](../README.md#compose-beside).

### Filesystem + Fetch wrap verify (E3 / #23)

Scripted, no-TTY check that the Cursor launchers attach and pin:

```bash
./scripts/verify-filesystem-fetch.sh
```

Uses `deadbugz-guard approve` / `--approve` / `--approve-file`, asserts pin mode `0600`, and a later `tools/list` allow when unchanged. CI runs the same job (`verify-filesystem-fetch` in `.github/workflows/ci.yml`). Local skip gates: [scripts/README.md](../scripts/README.md).

GitHub official MCP toolsets / `config_or_inventory_changed` is the E1 half of #23: [docs/github-toolsets.md](github-toolsets.md) and `./scripts/verify-github-toolsets.sh`. Not this script.
