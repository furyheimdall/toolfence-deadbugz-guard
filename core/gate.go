package core

import (
	"context"
	"errors"
	"sync"
)

// ErrSilentRepin is returned when a caller tries to overwrite an existing
// pin without apply_approval. Chief / issue #3: silent re-pin is forbidden.
var ErrSilentRepin = errors.New("core: silent re-pin is forbidden")

type pendingApproval struct {
	Candidate Pin
	Diff      *ToolDiffSummary
}

// MemoryGate is a thin in-process Gate for HITL/audit tests. E1 replaces
// the pin store and production Evaluate; the public method names stay
// Evaluate and ApplyApproval.
type MemoryGate struct {
	mu      sync.Mutex
	pin     *Pin
	pending *pendingApproval
	seq     int
}

// NewMemoryGate returns an empty fail-closed gate (PIN_MISSING until an
// initial pin is installed).
func NewMemoryGate() *MemoryGate {
	return &MemoryGate{}
}

// InstallInitialPin sets the first pin only. Later hash advances must go
// through ApplyApproval(approve).
func (g *MemoryGate) InstallInitialPin(p Pin) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.pin != nil {
		return ErrSilentRepin
	}
	g.pin = clonePin(p)
	g.seq = 1
	return nil
}

// CurrentPin returns a copy of the active pin, or nil if missing.
func (g *MemoryGate) CurrentPin() *Pin {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.pin == nil {
		return nil
	}
	return clonePin(*g.pin)
}

// PendingCandidate returns the live pin waiting for HITL, or nil.
func (g *MemoryGate) PendingCandidate() *Pin {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.pending == nil {
		return nil
	}
	return clonePin(g.pending.Candidate)
}

// Evaluate is fail-closed:
//   - no pin → PIN_MISSING
//   - pending re-approval → APPROVAL_PENDING (all tool calls denied; Chief F)
//   - aggregate match → Allow{pin_revision}
//   - otherwise → DIFF_NONEMPTY and arm pending candidate
func (g *MemoryGate) Evaluate(_ context.Context, live []ToolDef) GateDecision {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.pin == nil {
		return Deny(ReasonPinMissing, nil)
	}
	if g.pending != nil {
		return Deny(ReasonApprovalPending, copyDiff(g.pending.Diff))
	}

	g.seq++
	livePin := PinTools(live, nextPinVersion(g.pin.Version, g.seq))
	if livePin.Aggregate == g.pin.Aggregate {
		g.seq-- // no candidate consumed
		return Allow(g.pin.Version)
	}
	diff := summarize(*g.pin, livePin)
	g.pending = &pendingApproval{Candidate: livePin, Diff: &diff}
	return Deny(ReasonDiffNonempty, copyDiff(&diff))
}

// ApplyApproval consumes approve|deny for pin_revision.
//
// Approve (Chief G): pin becomes the pending candidate (newHash).
// Deny (Chief H): old pin is kept; hash does not advance.
func (g *MemoryGate) ApplyApproval(pinRevision string, outcome ApprovalOutcome) GateDecision {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.pending == nil {
		if g.pin == nil {
			return Deny(ReasonPinMissing, nil)
		}
		if outcome == OutcomeApprove {
			return Deny(ReasonInternalError, nil)
		}
		return Deny(ReasonApprovalDenied, nil)
	}
	if pinRevision != g.pending.Candidate.Version {
		// Fail-closed: do not advance; leave pending so all calls stay denied.
		return Deny(ReasonInternalError, copyDiff(g.pending.Diff))
	}
	diff := copyDiff(g.pending.Diff)
	switch outcome {
	case OutcomeApprove:
		g.pin = clonePin(g.pending.Candidate)
		g.pending = nil
		return Allow(g.pin.Version)
	case OutcomeDeny:
		g.pending = nil
		return Deny(ReasonApprovalDenied, diff)
	default:
		return Deny(ReasonInternalError, diff)
	}
}

func copyDiff(d *ToolDiffSummary) *ToolDiffSummary {
	if d == nil {
		return nil
	}
	out := *d
	out.Added = append([]string(nil), d.Added...)
	out.Removed = append([]string(nil), d.Removed...)
	out.Changed = append([]string(nil), d.Changed...)
	return &out
}
