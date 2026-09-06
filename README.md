# Deadbugz guard

Thin MCP sidecar/plugin that hash-pins approved tool definitions, diffs every `tools/list`, and fails closed until a human re-approves.

It sits **beside** the agent’s existing MCP path. It does not replace that path.

[Star](https://github.com/furyheimdall/toolfence-deadbugz-guard) · [Install](#install) · [Docs](docs/landing.md)

OSS pilot of the Deadbugz triangle: **pin → diff → fail-closed re-approval**, plus the minimum HITL and local audit hooks that path needs.

## Core loop

1. **Hash-pin** approved MCP tool definitions.
2. **Diff** the live `tools/list` against that pin on every listing.
3. On mismatch → **fail closed** and require **re-approval** before the new definitions are trusted.

HITL and audit exist only to support that re-approval path.

## Install

Packaging is not on `main` yet. The sidecar/plugin adapter is tracked in [#7](https://github.com/furyheimdall/toolfence-deadbugz-guard/issues/7); repo scaffold (layout, MIT, local compose) is [#2](https://github.com/furyheimdall/toolfence-deadbugz-guard/issues/2).

Until those land, clone the repo and follow the issue map below.

```bash
git clone https://github.com/furyheimdall/toolfence-deadbugz-guard.git
cd toolfence-deadbugz-guard
```

## MVP (IN)

- hash-pin of tool definitions
- `tools/list` diff against pin
- fail-closed until re-approval
- minimal HITL hook
- minimal local audit (JSONL)
- ship as sidecar/plugin (Obot / Docker / Lunar-style); nearest OSS reference: **AGT**

## OUT

- full self-host firewall / stdio gateway product
- SaaS / billing
- prompt guardrails
- K8s full mesh

## Not this

Deadbugz guard is a **thin sidecar/plugin**, not a product category swap-in.

| This is | This is not |
| --- | --- |
| A pin → diff → fail-closed check on `tools/list` | A multi-server MCP gateway or stdio multiplexer you run as the agent’s front door |
| A plugin that sits beside the host’s existing MCP path | A control plane, policy mesh, or “approve every tool call” broker |
| HITL + local JSONL only for re-pin / re-approval | Prompt filtering, jailbreak scoring, or content guardrails |
| Local, self-hosted OSS | A SaaS quote flow, tenant console, or billed SKU |

If a change needs a full gateway, remote SIEM, or prompt-layer product, it is out of scope here. Open an issue first; do not send that as a drive-by PR.

## Docs

- [Landing](docs/landing.md) — positioning for the OSS pilot
- [Security](SECURITY.md) — threat model for the MVP loop
- [Contributing](CONTRIBUTING.md) — scope rules and PR checklist

## License

MIT

## Tracking

- Epic: [#1](https://github.com/furyheimdall/toolfence-deadbugz-guard/issues/1)
- [#2](https://github.com/furyheimdall/toolfence-deadbugz-guard/issues/2) Scaffold (E3)
- [#3](https://github.com/furyheimdall/toolfence-deadbugz-guard/issues/3) hash-pin (E1)
- [#4](https://github.com/furyheimdall/toolfence-deadbugz-guard/issues/4) tools/list diff (E1)
- [#5](https://github.com/furyheimdall/toolfence-deadbugz-guard/issues/5) fail-closed gate (E1)
- [#6](https://github.com/furyheimdall/toolfence-deadbugz-guard/issues/6) HITL hook (E2)
- [#7](https://github.com/furyheimdall/toolfence-deadbugz-guard/issues/7) sidecar/plugin + smoke (E3)
- [#8](https://github.com/furyheimdall/toolfence-deadbugz-guard/issues/8) audit JSONL (E2)
