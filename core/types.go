package core

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// ReasonCode is the issue #5 vocabulary. Callers must not invent aliases
// (no GateStatus.Code / DIFF_DETECTED / APPROVAL_REQUIRED names).
type ReasonCode string

const (
	ReasonOK              ReasonCode = "OK"
	ReasonDiffNonempty    ReasonCode = "DIFF_NONEMPTY"
	ReasonApprovalDenied  ReasonCode = "APPROVAL_DENIED"
	ReasonApprovalPending ReasonCode = "APPROVAL_PENDING"
	ReasonPinMissing      ReasonCode = "PIN_MISSING"
	ReasonInternalError   ReasonCode = "INTERNAL_ERROR"
)

// ApprovalOutcome is the apply_approval decision consumed by the gate.
type ApprovalOutcome string

const (
	OutcomeApprove ApprovalOutcome = "approve"
	OutcomeDeny    ApprovalOutcome = "deny"
)

// ToolDef is one MCP tools/list entry. Pin hash covers name + description +
// inputSchema (+ annotations when present). Extra fields are ignored.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
	Annotations json.RawMessage `json:"annotations,omitempty"`
}

// Pin is the candidate / active hash-pin. Aggregate is the SHA-256 hex
// (newHash / oldHash). Version is pin_revision.
type Pin struct {
	Version    string            `json:"version"`
	Aggregate  string            `json:"aggregate"`
	ToolHashes map[string]string `json:"tool_hashes,omitempty"`
	Tools      []ToolDef         `json:"tools,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
}

// ToolDiffSummary is the HITL request payload and Deny.diff (issue #5).
// Tool names only — never raw schemas or call arguments.
type ToolDiffSummary struct {
	Added       []string `json:"added"`
	Removed     []string `json:"removed"`
	Changed     []string `json:"changed"`
	PinRevision string   `json:"pin_revision"`
	LiveHash    string   `json:"live_hash"`
	PinHash     string   `json:"pin_hash"`
}

// Empty reports whether the tools/list diff has no added/removed/changed names.
func (s ToolDiffSummary) Empty() bool {
	return len(s.Added) == 0 && len(s.Removed) == 0 && len(s.Changed) == 0
}

// GateDecision is Allow{pin_revision} | Deny{reason_code, diff}.
type GateDecision struct {
	Allowed     bool             `json:"allowed"`
	PinRevision string           `json:"pin_revision,omitempty"`
	ReasonCode  ReasonCode       `json:"reason_code,omitempty"`
	Diff        *ToolDiffSummary `json:"diff,omitempty"`
}

// Allow is the #5 allow path (ReasonCode OK).
func Allow(pinRevision string) GateDecision {
	return GateDecision{
		Allowed:     true,
		PinRevision: pinRevision,
		ReasonCode:  ReasonOK,
	}
}

// Deny is the #5 deny path.
func Deny(code ReasonCode, diff *ToolDiffSummary) GateDecision {
	rev := ""
	if diff != nil {
		rev = diff.PinRevision
	}
	return GateDecision{
		Allowed:     false,
		PinRevision: rev,
		ReasonCode:  code,
		Diff:        diff,
	}
}

// Approver is the #6 injection interface. Request payload is ToolDiffSummary
// plus the candidate pin (newHash lives on candidate.Aggregate).
//
// On approve: return candidate.Version and nil.
// On deny: return "" and a non-nil error.
// On timeout: return "" and a non-nil error (context deadline or
// ErrApprovalTimeout). MapApproverResult treats timeout as deny (Chief H).
type Approver interface {
	RequestApproval(ctx context.Context, diff ToolDiffSummary, candidate Pin) (approvedVersion string, err error)
}

// ErrApprovalTimeout is the Approver sentinel for a timed-out HITL wait.
// ApplyApproval / MapApproverResult treat it as deny (Chief H).
var ErrApprovalTimeout = errors.New("core: approval timeout")

// MapApproverResult is apply_approval's Approver wiring: timeout, deny, or
// version mismatch → OutcomeDeny (old pin kept). Matching version → approve.
func MapApproverResult(approvedVersion string, err error, candidateVersion string) ApprovalOutcome {
	if err != nil || approvedVersion == "" || approvedVersion != candidateVersion {
		return OutcomeDeny
	}
	return OutcomeApprove
}

// ApproverDecisionLabel is the audit "decision" field (approve|deny|timeout).
func ApproverDecisionLabel(err error) string {
	if err == nil {
		return "approve"
	}
	if errors.Is(err, ErrApprovalTimeout) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "timeout"
	}
	return "deny"
}

// Auditor is the #8 injection interface. Implementations must persist only
// redaction-safe fields (see audit.AllowedFields).
type Auditor interface {
	Record(event string, fields map[string]any)
}

// Gate is the #5 surface HITL wires into.
type Gate interface {
	Evaluate(ctx context.Context, live []ToolDef) GateDecision
	ApplyApproval(pinRevision string, outcome ApprovalOutcome) GateDecision
	CurrentPin() *Pin
	PendingCandidate() *Pin
}
