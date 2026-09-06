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
//     PIN_MISSING | INTERNAL_ERROR
//   - Evaluate(live) / ApplyApproval(pin_revision, approve|deny)
//   - Approver.RequestApproval(ctx, diff, candidate Pin)
//   - Auditor.Record(event, fields)
//
// Pin = SHA-256 of tools sorted by name, then canonical JSON of name +
// description + inputSchema (+ annotations). Evaluate never writes a pin.
// Missing or tampered pin is PIN_MISSING. Silent re-pin is forbidden:
// only InstallInitialPin (first pin) or ApplyApproval(approve) persist.
package core
