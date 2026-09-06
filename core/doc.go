// Package core is the Deadbugz fail-closed gate: hash-pin of MCP tool
// definitions, tools/list diff against that pin, and re-approval.
//
// Public names (issue #5 / merged #12):
//
//   - ToolDiffSummary, GateDecision, ReasonCode
//     (OK | DIFF_NONEMPTY | APPROVAL_DENIED | APPROVAL_PENDING |
//     PIN_MISSING | INTERNAL_ERROR)
//   - Evaluate / ApplyApproval
//   - Approver / Auditor
//   - Pin, ToolDef, PinTools
//
// Pin = SHA-256 of tools sorted by name, then canonical JSON of name +
// description + inputSchema (+ annotations). Evaluate never writes a pin.
// Missing or tampered pin is PIN_MISSING. Silent re-pin is forbidden:
// only InstallInitialPin (first pin) or ApplyApproval(approve) persist.
//
// HITL (#6) and audit (#8) inject Approver and Auditor; this package does
// not own the JSONL writer or HITL UI.
package core
