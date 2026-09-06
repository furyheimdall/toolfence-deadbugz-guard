# Demo: allow vs deny

Deadbugz guard pins an approved `tools/list`. A matching listing is forwarded. A mutated listing fails closed until a human re-approves.

![benign → allow, poison → deny](assets/smoke-allow-deny.svg)

That figure is the smoke pair: **benign (pinned) → allow**, **poison → deny**. Reorder-only still allows. Add / remove fail closed. See [README Smoke test](../README.md#smoke-test).

## Re-run smoke

From the repo root, with Go 1.22+:

```bash
./scripts/smoke.sh
```

The script runs the sidecar / mock-MCP tests that pin a **benign** listing, then flip `FLIP_PATH` (atomic `mv` only) through reorder / poison / add / remove / deadbugz. Expected tail:

```
PASS  benign (pinned) → allow
PASS  reorder → allow
PASS  poison → deny
PASS  add / remove → fail-closed
PASS  deadbugz + call_gate=3 → block after gate
ok
```

To watch the wrap yourself (stdio, not a GIF recorder):

```bash
go build -o bin/deadbugz-guard ./cmd/deadbugz-guard
go build -o bin/mock-mcp-deadbugz ./cmd/mock-mcp-deadbugz
./bin/deadbugz-guard --write-pin --pin testdata/pin.json --from-mode benign
./bin/deadbugz-guard --pin testdata/pin.json --audit testdata/audit.jsonl \
  --hitl http://127.0.0.1:8765 --call-gate 3 -- ./bin/mock-mcp-deadbugz
```

Leave `testdata/flip.json` at `"mode": "benign"` for allow. Flip to `"mode": "poison"` with temp write + `mv` (`mockmcp.WriteFlip`) to see deny.

No hosted demo, quote form, or SaaS replay. This is the local OSS loop only.
