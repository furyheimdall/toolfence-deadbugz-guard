# Deadbugz guard

**Pin the tools you approved. Diff every `tools/list`. Fail closed until a human re-approves.**

Deadbugz guard is a thin MCP sidecar/plugin. It sits beside the agent host’s existing MCP path and watches one thing: whether the tool definitions the host is about to trust still match the pin you signed off on.

[Star the repo](https://github.com/furyheimdall/toolfence-deadbugz-guard) · [Install](../README.md#install) · [Docs](../README.md#docs)

This page is the OSS landing for the pilot. There is no product SKU, trial, or quote form.

## The loop

```
approved tools/list  →  hash-pin
                            │
              every tools/list
                            │
                     compare to pin
                      /          \
                 match            mismatch
                   │                  │
              listing ok         fail closed
                                      │
                                 human review
                                      │
                           approve → new pin
                           deny    → stay closed
```

HITL is a re-approval hook (diff summary in, approve or deny out). Audit is a local JSONL trail of pin, diff, block, and decision events. Neither is a separate product.

## Why a sidecar

Agents already speak MCP. Deadbugz guard does not ask you to rip that out.

- The host keeps its current MCP connection.
- The guard plugs in on the `tools/list` path (Obot / Docker / Lunar-style plugin; nearest OSS reference: AGT).
- Unchanged listings pass. Mutated listings fail closed until you re-pin.

That is the whole MVP. See the [README](../README.md) for the engineering IN/OUT list and the issue map (#1–#8).

## Not this

Deadbugz guard is **not**:

- A full stdio or multi-server MCP gateway you put in front of every agent
- A control tower, policy mesh, or “approve every call” broker
- A prompt-guardrail or jailbreak filter
- A SaaS console, billed plan, or “request a quote” product

Those are out of scope for this repository. Contributions that turn the sidecar into any of the above will be closed. Details: [CONTRIBUTING](../CONTRIBUTING.md).

## Scope, in one table

| In (MVP) | Out |
| --- | --- |
| Hash-pin of tool definitions | Full self-host firewall / stdio gateway product |
| `tools/list` diff against pin | SaaS / billing |
| Fail-closed until re-approval | Prompt guardrails |
| Minimal HITL hook | K8s full mesh |
| Minimal local audit (JSONL) | Remote SIEM / cloud shipping |
| Sidecar / plugin packaging | Multi-tenant approval broker |

## Security

The threat we take on: an MCP server (or anything that can change its advertised tools) silently adds, removes, or rewrites a tool definition after you approved the previous set.

The threat we do **not** take on: argument inspection, prompt injection, transport multiplexing, or cluster identity.

Read [SECURITY.md](../SECURITY.md) for trust boundaries, the control table, and fail-closed states.

## Get involved

1. Star [furyheimdall/toolfence-deadbugz-guard](https://github.com/furyheimdall/toolfence-deadbugz-guard) if the loop is useful.
2. Install when [#2](https://github.com/furyheimdall/toolfence-deadbugz-guard/issues/2) and [#7](https://github.com/furyheimdall/toolfence-deadbugz-guard/issues/7) land — clone today and follow the issue map.
3. Pick a [good first issue](../CONTRIBUTING.md#good-first-issues): hash-pin (#3), `tools/list` diff (#4), or local audit JSONL (#8).

MIT licensed. Implementation is in progress; the contract above is what we will ship first.
