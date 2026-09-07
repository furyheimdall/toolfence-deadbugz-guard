// Package core is the Deadbugz fail-closed gate: hash-pin of MCP tool
// definitions, tools/list diff against that pin, and re-approval (#3 #4 #5).
//
// Layout (locked with #2): this is the top-level `core/` package. Room is
// left for `hitl/`, `audit/`, and `sidecar/` — those plus LICENSE, Docker
// Compose, and smoke are owned by other issues, not this package.
//
// Public names (issue #5 / merged #12), so HITL/audit/sidecar can depend:
//
//   - ToolDef, Pin (Version, Aggregate, ToolHashes)
//   - PinTools, Diff, HashCatalog
//   - ToolDiffSummary { added, removed, changed, pin_revision, live_hash, pin_hash }
//   - GateDecision = Allow{pin_revision} | Deny{reason_code, diff}
//   - ReasonCode: OK | DIFF_NONEMPTY | APPROVAL_DENIED | APPROVAL_PENDING |
//     PIN_MISSING | INTERNAL_ERROR | config_or_inventory_changed |
//     tools_list_drift (#21)
//   - Evaluate(live) / ApplyApproval(pin_revision, approve|deny)
//   - Approver.RequestApproval(ctx, diff, candidate Pin)
//   - Auditor.Record(event, fields) — approved/denied include who/when/oldHash/newHash
//
// G: approve advances pin to newHash only; emit approved with hash transition.
// H: deny or Approver timeout → APPROVAL_DENIED; old pin kept; calls stay denied.
//
// Pin = SHA-256 of tools sorted by name, then canonical JSON of name +
// description + inputSchema (+ annotations). Evaluate never writes a pin.
// Missing or tampered pin is PIN_MISSING. Silent re-pin is forbidden:
// only InstallInitialPin (first pin) or ApplyApproval(approve) persist.
//
// Issue #21: config_fingerprint = hash(stable(argv after --) || subset(env)).
// pin_id = hash(server_name || config_fingerprint || canonical_tools_hash).
// Same fingerprint + live tools/list mutate → tools_list_drift (never
// auto-promote). Fingerprint / toolsets / argv / env / credential-scope
// change → config_or_inventory_changed (re-approval even if an old tools
// hash matches). Reorder-only under the same fingerprint still Allows.
package core
