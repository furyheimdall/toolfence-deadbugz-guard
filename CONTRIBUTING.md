# Contributing to Deadbugz guard

Thanks for helping. This repo is an OSS **pilot** of a thin MCP sidecar/plugin. The product is the Deadbugz triangle only:

1. Hash-pin approved tool definitions
2. Diff every `tools/list`
3. Fail closed + re-approval on mismatch

HITL and audit PRs are welcome only when they serve that re-approval path.

Please read the [README](README.md) MVP **IN** / **OUT** lists and epic [#1](https://github.com/furyheimdall/toolfence-deadbugz-guard/issues/1) before writing code.

## Keep the MVP sharp

In scope (see README):

- hash-pin of tool definitions
- `tools/list` diff against pin
- fail-closed until re-approval
- minimal HITL hook
- minimal local audit (JSONL)
- ship as sidecar/plugin (Obot / Docker / Lunar-style)

Out of scope (do not send these as the PR’s main change):

- full self-host firewall / stdio gateway product
- SaaS / billing
- prompt guardrails
- K8s full mesh

Also out, even if they look “related”:

- A multi-server MCP front door or stdio multiplexer as the product
- Remote SIEM / cloud log shipping
- Fancy approval UI or a multi-tenant broker
- Prompt filters, jailbreak scorers, or “agent safety” layers

If you believe a larger design is needed, open an issue and wait. Do not grow the sidecar into a gateway in a drive-by PR.

## Drive-by PRs we will close

Please do **not** open PRs that:

- Rebrand Deadbugz guard as a gateway, control plane, or “MCP firewall”
- Add a full stdio / multi-server MCP proxy “while we’re here”
- Add pricing pages, quote CTAs, or SaaS tenant plumbing
- Add prompt-guardrail product framing or filter pipelines
- Replace the README IN/OUT lists or drop the issue map
- Copy proprietary code (including from AGT). A short mapping *note* is fine; a port is not.

Small docs fixes, tests for the core loop, and slices that match an existing issue are the right size.

## Good first issues

Start here — they match the MVP loop and do not require a gateway:

| Issue | Why it is a good first slice |
| --- | --- |
| [#3](https://github.com/furyheimdall/toolfence-deadbugz-guard/issues/3) hash-pin | Canonical serialize + local pin store; unit-testable |
| [#4](https://github.com/furyheimdall/toolfence-deadbugz-guard/issues/4) `tools/list` diff | Added / removed / changed / reorder-only cases |
| [#8](https://github.com/furyheimdall/toolfence-deadbugz-guard/issues/8) audit JSONL | Append-only local events; no remote backend |

After those, [#5](https://github.com/furyheimdall/toolfence-deadbugz-guard/issues/5) (fail-closed gate) and [#6](https://github.com/furyheimdall/toolfence-deadbugz-guard/issues/6) (HITL hook) are the next core slices. [#2](https://github.com/furyheimdall/toolfence-deadbugz-guard/issues/2) and [#7](https://github.com/furyheimdall/toolfence-deadbugz-guard/issues/7) are scaffold / adapter work — coordinate so we do not get two layouts.

Comment on the issue before starting so work does not overlap.

## Development

Language is **Go**. Match the merged `core` / `hitl` / `audit` contract from [#12](https://github.com/furyheimdall/toolfence-deadbugz-guard/pull/12). Do not redefine those seats. The sidecar wrap lives in `sidecar/` and `cmd/deadbugz-guard` ([#7](https://github.com/furyheimdall/toolfence-deadbugz-guard/issues/7)).

- Prefer tests that lock the loop: same input → same hash; no-change pass; mutation fail-closed; approve binds one pin revision.
- Do not add network services that the MVP issues did not ask for.
- Keep examples as sidecar/plugin config (compose / host plugin), not as a standalone multi-server gateway.

## Pull request checklist

Copy this into the PR body and check what applies:

- [ ] PR title names the MVP slice (pin, diff, fail-closed, HITL, audit, sidecar)
- [ ] Linked to an existing issue (`#3`–`#8` or epic `#1`)
- [ ] Change stays inside README **IN**; nothing from **OUT** is introduced as a feature
- [ ] Docs still say **Deadbugz guard** (thin sidecar/plugin). No gateway / firewall hero copy
- [ ] README **MVP (IN)**, **OUT**, and **Tracking** issue map are still present if you touched `README.md`
- [ ] HITL / audit work is limited to the re-approval path
- [ ] Tests cover the new behavior (or the issue is docs-only)
- [ ] No secrets, proprietary copies, or SaaS quote / pricing CTAs
- [ ] Fail-closed remains the default on mismatch or uncertainty

## License

By contributing you agree your work is licensed under the repository’s MIT license.
