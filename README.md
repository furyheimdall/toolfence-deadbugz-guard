# toolfence-deadbugz-guard

OSS pilot: **Deadbugz triangle** guard as a self-host sidecar/plugin.

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

## Package layout (Go MVP)

`core` (E1 gate types + thin `MemoryGate` adapter), `hitl` (#6), `audit` (#8). Sidecar is E3.

Public gate vocabulary (issue #5, E1 accepted): `ToolDiffSummary`, `GateDecision`, `ReasonCode` (`OK`, `DIFF_NONEMPTY`, `APPROVAL_DENIED`, `APPROVAL_PENDING`, `PIN_MISSING`, `INTERNAL_ERROR`). `Approver.RequestApproval` takes a `ToolDiffSummary` and candidate pin. `ApplyApproval(pin_revision, approve|deny)` returns `GateDecision`. Timeout is deny.

## HITL stub (#6)

Local only — callback, CLI, or tiny HTTP stub. No SaaS.

```bash
# terminal 1 — stub
go run ./cmd/hitl serve -listen 127.0.0.1:8765

# terminal 2 — blocks on ToolDiffSummary + candidate pin (timeout = deny / Chief H)
go run ./cmd/hitl request -base http://127.0.0.1:8765 -timeout 60

# terminal 3 — decide (Chief G: approve advances newHash; deny keeps old pin)
go run ./cmd/hitl decide -base http://127.0.0.1:8765 -decision approve -who alice
# or:  go run ./cmd/hitl decide -decision deny -who alice
```

Sidecar/tests inject `hitl.CallbackApprover` or the same `hitl.Server` as `core.Approver`. Approve → `ApplyApproval(..., approve)` and pin becomes the candidate hash. Deny/timeout → `ApplyApproval(..., deny)` and the old pin is kept.

## Audit CLI (#8)

Append-only JSONL. Events: `pin_created`, `diff_detected`, `blocked`, `approved`, `denied`. Hash transitions record `oldHash` / `newHash`; re-approve also records `who` / `when`.

Redaction-safe allowlist (only these keys are written): `event`, `when`, `who`, `oldHash`, `newHash`, `pin_revision`, `pin_hash`, `live_hash`, `added`, `removed`, `changed`, `reason_code`, `decision`, `approved_version`. No secrets, raw tool args, or `inputSchema`.

```bash
go test ./...
go run ./cmd/audit -path ./audit.jsonl tail -n 20
```

