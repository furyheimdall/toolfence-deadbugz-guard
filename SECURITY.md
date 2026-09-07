# Security

Threat model for **Deadbugz guard** MVP only: hash-pin approved MCP tool definitions, diff every `tools/list`, fail closed until re-approval.

HITL and local audit are in scope only as far as that re-approval path needs them.

This is not a threat model for a multi-server MCP gateway, a host-wide policy plane, or prompt-layer controls.

## In scope

| Asset / event | Why it matters |
| --- | --- |
| Canonical tool-definition bytes and their hashes | A pin that is unstable or forgeable cannot detect change |
| Pin store (local file for MVP) | If an attacker rewrites the pin, mismatch never fires |
| Live `tools/list` snapshot used for diff | The listing the host would hand the agent is the comparison input |
| Fail-closed gate | Mismatch must block use of the new listing until a human approves |
| Re-approval decision (approve / deny) | Approval is the only way a new pin revision becomes trusted |
| Local append-only JSONL for pin / diff / approval events | Operators need a local record of why the gate opened or stayed shut |

## Out of scope

The following are **not** modeled or claimed as protected by this MVP:

- A full stdio / multi-server MCP front door, request proxy, or host multiplexer
- Per-`tools/call` argument policy, rate limits, or data-loss controls
- Prompt injection, jailbreak, or model-output filtering
- Remote SIEM, cloud log shipping, or multi-tenant audit
- Kubernetes mesh, service identity, or cluster admission
- SaaS tenancy, billing, or a hosted approval console
- Integrity of the MCP server *implementation* beyond the listed tool definitions
- Host, container, or OS hardening of the agent runtime

If you need those, they belong in another component. Deadbugz guard does not stand in for them.

## Trust boundaries

```
  operator / approver
          |
          |  create pin, approve or deny a new revision
          v
  +-------------------+     pin file + JSONL      +------------------+
  | Deadbugz guard    | <-----------------------> | local disk       |
  | sidecar / plugin  |                            | (pin, audit)     |
  +-------------------+                            +------------------+
          ^        |
          |        |  allow listing  /  fail closed
          |        v
  +-------------------+     tools/list            +------------------+
  | agent host        | <-----------------------> | MCP server(s)    |
  | (existing path)   |                            | the host already |
  +-------------------+                            | talks to         |
  +-------------------+                            +------------------+
```

Assumptions for MVP:

- The **operator** who creates the first pin and later re-approves is trusted.
- The **agent host** is trusted to invoke Deadbugz guard on the stdio wrap path and to honor a fail-closed result. It is **not** trusted to refresh after `notifications/tools/list_changed`, to load deferred tools correctly, or to treat those events as authoritative. If the host bypasses the sidecar, the pin does not apply.
- The **MCP server** is *not* trusted to keep tool definitions stable. Silent add / remove / schema change is the primary threat this loop addresses.
- **Local disk** that holds the pin and JSONL is trusted at the same level as the host. Pin-store tamper is in scope as a failure mode; full disk encryption and remote attestation are not.
- There is **no** cloud control plane and **no** third-party approval broker in the MVP trust set.

## Controls

| Control | What it does | MVP limit |
| --- | --- | --- |
| Canonical serialize + hash-pin | Stable hash of approved tool defs ([#3](https://github.com/furyheimdall/toolfence-deadbugz-guard/issues/3)) | Local file store; no remote pin registry |
| `tools/list` diff | Added / removed / changed tools vs pin ([#4](https://github.com/furyheimdall/toolfence-deadbugz-guard/issues/4)) | Listing path only; not `tools/call` |
| Hash-before-forward | Every live listing (host `tools/list` and post-`list_changed` refresh) is hashed/diffed **before** the host sees it ([#22](https://github.com/furyheimdall/toolfence-deadbugz-guard/issues/22)) | stdio wrap only; not a remote HTTP proxy |
| Fail-closed gate | Default deny when the diff is non-empty ([#5](https://github.com/furyheimdall/toolfence-deadbugz-guard/issues/5)) | Host must not bypass the gate |
| HITL re-approval | Human sees a diff summary; approve binds a new pin revision ([#6](https://github.com/furyheimdall/toolfence-deadbugz-guard/issues/6)) | Local callback / CLI / HTTP stub; no SaaS |
| Local JSONL audit | Append-only events: pin_created, diff_detected, blocked, approved, denied ([#8](https://github.com/furyheimdall/toolfence-deadbugz-guard/issues/8)) | Local tail only; no remote shipping |

## Fail-closed states

Deadbugz guard stays shut (listing not trusted / not forwarded as approved) when any of the following hold:

| State | Gate | Clears when |
| --- | --- | --- |
| No pin exists yet | Closed | Operator creates an initial pin from a reviewed `tools/list` |
| Live listing hash ≠ pinned hash | Closed | Human approves; gate opens only for that approved pin revision |
| Diff is empty except reorder, but canonicalization is broken | Treat as mismatch (closed) | Canonical serialize is fixed and hashes match, or a human re-approves after review |
| Pin file missing, unreadable, or fails integrity checks the MVP defines | Closed | Pin is restored or a new pin is created through the approval path |
| HITL endpoint unreachable when a mismatch requires a decision | Closed | Approver becomes reachable and records approve or deny |
| Deny recorded for a candidate revision | Closed | A later, distinct revision is approved (deny does not open the gate) |
| Audit log cannot be appended (if the implementation requires it for the decision) | Closed | Local log path is writable again |

Fail-open on “diff engine crashed” or “audit disk full” is **not** an MVP behavior. Ambiguity defaults to closed.

## Host `list_changed` is not a security boundary

Claude Code and other hosts have had bugs around `notifications/tools/list_changed` and deferred-tool refresh (skipped re-list, stale cache, treating the notification as authoritative). Those bugs must not weaken the gate.

Deadbugz guard hashes and diffs every live `tools/list` — including a guard-owned refresh after `list_changed` — **before** forwarding a listing to the host. Mid-session poison / add / remove stays fail-closed on the next inventory the wrap observes (typically the call-gate `tools/list` sync) without waiting for the client to refresh. A host `list_changed` or deferred-tool reload is ignored as a security event.

Re-approval never means “allow this listing this once.” It means “this exact pin revision is now the trusted pin.”

## What this does not claim

- It does not prove the MCP server is safe to call.
- It does not inspect prompts, completions, or tool arguments.
- It does not terminate or multiplex MCP transports.
- It does not replace host authentication, secrets handling, or supply-chain review of the sidecar binary itself.

## Vulnerability reporting

TBD.

Preferred path once a contact is published: privately report pin-bypass, fail-open defaults, hash-canonicalization collisions that hide a real tool change, or approval-binding bugs that accept a different revision than the one reviewed.

Do not file those as public issues until a reporting address or security policy contact is listed here.
