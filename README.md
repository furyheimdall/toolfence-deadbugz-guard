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
