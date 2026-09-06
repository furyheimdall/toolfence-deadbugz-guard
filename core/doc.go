// Package core is the shared MVP contract for the Deadbugz fail-closed gate.
//
// E1 owns production hash-pin / tools/list diff / Evaluate. This package
// publishes the types E1 accepted on issue #5 so HITL (#6) and audit (#8)
// can compile and be tested independently:
//
//   - ToolDiffSummary
//   - GateDecision = Allow{pin_revision} | Deny{reason_code, diff}
//   - ReasonCode: DIFF_NONEMPTY | APPROVAL_DENIED | APPROVAL_PENDING |
//     PIN_MISSING | INTERNAL_ERROR (+ OK on the allow path)
//   - Evaluate(live tools) GateDecision
//   - ApplyApproval(pin_revision, approve|deny) GateDecision
//
// Approver and Auditor are injection points implemented by the hitl and
// audit packages. MemoryGate is a thin in-process adapter for tests — not
// the production pin store.
package core
