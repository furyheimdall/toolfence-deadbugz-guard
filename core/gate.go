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

// MemoryGate is the production fail-closed gate (#3/#4/#5) with an optional
// file pin store. Public names stay Evaluate / ApplyApproval / CurrentPin /
// PendingCandidate so HITL (#6) and audit (#8) keep compiling.
type MemoryGate struct {
	mu      sync.Mutex
	pin     *Pin
	pending *pendingApproval
	seq     int
	store   *FileStore
}

// NewMemoryGate returns an empty fail-closed gate (PIN_MISSING until an
// initial pin is installed).
func NewMemoryGate() *MemoryGate {
	return &MemoryGate{}
}

// NewFileGate is the production pin store. A missing or tampered file is
// fail-closed; Evaluate never creates the file.
func NewFileGate(path string) *MemoryGate {
	return &MemoryGate{store: &FileStore{Path: path}}
}

// InstallInitialPin sets the first pin only. Later hash advances must go
// through ApplyApproval(approve). Evaluate never calls this.
func (g *MemoryGate) InstallInitialPin(p Pin) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.pin != nil {
		return ErrSilentRepin
	}
	if g.store != nil {
		if existing, err := g.store.Load(); err == nil && existing != nil {
			return ErrSilentRepin
		} else if err != nil && !errors.Is(err, errPinMissing) && !errors.Is(err, errPinTampered) {
			return err
		}
		if err := g.store.Save(p); err != nil {
			return err
		}
	}
	g.pin = clonePin(p)
	g.seq = 1
	g.pending = nil
	return nil
}

// CurrentPin returns a copy of the active pin, or nil if missing.
func (g *MemoryGate) CurrentPin() *Pin {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.refreshFromStore(); err != nil {
		return nil
	}
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
//   - no pin / tampered pin → PIN_MISSING (no silent re-pin)
//   - pending re-approval → APPROVAL_PENDING (all tool calls denied; Chief F)
//   - aggregate match → Allow{pin_revision}
//   - add / remove / description or schema change → DIFF_NONEMPTY
func (g *MemoryGate) Evaluate(_ context.Context, live []ToolDef) GateDecision {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.refreshFromStore(); err != nil {
		g.pin = nil
		return Deny(ReasonPinMissing, nil)
	}
	if g.pin == nil {
		return Deny(ReasonPinMissing, nil)
	}
	if g.pending != nil {
		return Deny(ReasonApprovalPending, copyDiff(g.pending.Diff))
	}

	g.seq++
	livePin, err := HashCatalog(live, nextPinVersion(g.pin.Version, g.seq))
	if err != nil {
		g.seq--
		return Deny(ReasonInternalError, nil)
	}
	if livePin.Aggregate == g.pin.Aggregate {
		g.seq-- // no candidate consumed
		return Allow(g.pin.Version)
	}
	diff := summarize(*g.pin, livePin)
	g.pending = &pendingApproval{Candidate: livePin, Diff: &diff}
	return Deny(ReasonDiffNonempty, copyDiff(&diff))
}

// AuthorizeCall re-evaluates the current tools/list before every call
// (Chief E / J). Pending, mismatch, poison, or missing pin deny the call.
func (g *MemoryGate) AuthorizeCall(ctx context.Context, toolName string, live []ToolDef) GateDecision {
	_ = toolName
	return g.Evaluate(ctx, live)
}

// ApplyApproval consumes approve|deny for pin_revision.
//
// Approve (Chief G): pin becomes the pending candidate (newHash).
// Deny (Chief H): old pin is kept; hash does not advance.
func (g *MemoryGate) ApplyApproval(pinRevision string, outcome ApprovalOutcome) GateDecision {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.pending == nil {
		if err := g.refreshFromStore(); err != nil || g.pin == nil {
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
		if g.store != nil {
			if err := g.store.Save(*g.pin); err != nil {
				return Deny(ReasonInternalError, diff)
			}
		}
		return Allow(g.pin.Version)
	case OutcomeDeny:
		g.pending = nil
		return Deny(ReasonApprovalDenied, diff)
	default:
		return Deny(ReasonInternalError, diff)
	}
}

func (g *MemoryGate) refreshFromStore() error {
	if g.store == nil {
		return nil
	}
	p, err := g.store.Load()
	if err != nil {
		return err
	}
	g.pin = p
	return nil
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
