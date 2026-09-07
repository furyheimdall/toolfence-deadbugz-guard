# GitHub official MCP toolsets / inventory change

Part of [#23](https://github.com/furyheimdall/toolfence-deadbugz-guard/issues/23) (parent [#19](https://github.com/furyheimdall/toolfence-deadbugz-guard/issues/19)). Fingerprint / reason codes are [#21](https://github.com/furyheimdall/toolfence-deadbugz-guard/issues/21).

Official [`github-mcp-server`](https://github.com/github/github-mcp-server) advertises different tools when **toolsets** change. Deadbugz treats that as an **intentional config change**, not silent `tools/list` drift.

| Change | `config_fingerprint` | Evaluate reason | Policy |
| --- | --- | --- | --- |
| `--toolsets` / `--tools` / `GITHUB_TOOLSETS` / `GITHUB_TOOLS` / `DEADBUGZ_CREDENTIAL_SCOPE` | changes | `config_or_inventory_changed` | Expected re-approval (even if the old tools hash still matches) |
| Same fingerprint, live `tools/list` mutates | same | `tools_list_drift` | Fail-closed, never auto-promote |
| CSV reorder only (`repos,issues` vs `issues,repos`) | same | `OK` | Allow |

Tokens (`GITHUB_PERSONAL_ACCESS_TOKEN`, …) are **not** hashed.

Attach shape (copy-paste: [`examples/cursor.mcp.github.json`](../examples/cursor.mcp.github.json)):

```json
{
  "mcpServers": {
    "github": {
      "command": "/ABS/bin/deadbugz-guard",
      "args": [
        "--name", "github",
        "--pin", "~/.deadbugz/pins/github.json",
        "--",
        "/ABS/github-mcp-server",
        "--toolsets", "repos,issues"
      ],
      "env": {
        "DEADBUGZ_SERVER_NAME": "github",
        "GITHUB_TOOLSETS": "repos,issues"
      }
    }
  }
}
```

Put the host-provided GitHub token in the host secret store / `mcp.json` `env`. Do not put it in the pin. Changing `GITHUB_TOOLSETS` or `--toolsets` after the pin exists fails closed until HITL / `deadbugz-guard approve` / `--approve-file`.

## Scripted verify (CI, no GitHub credentials)

```bash
./scripts/verify-github-toolsets.sh
```

Wraps the official attach shape around `mock-mcp-deadbugz` (`scripts/fixtures/github-mcp-server`). The live official binary is **skipped** unless you opt in with `DEADBUGZ_LIVE_GITHUB_MCP=1` **and** `GITHUB_PERSONAL_ACCESS_TOKEN`. CI does not set those.

Filesystem + Fetch wrap verify is a separate #23 slice (E3 / PR #28). This document does not cover that path.
