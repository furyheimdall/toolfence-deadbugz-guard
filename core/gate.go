package core

import (
	"context"
	"errors"
	"sync"
	"time"
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
	mu       sync.Mutex
	pin      *Pin
	pending  *pendingApproval
	seq      int
	store    *FileStore
	approver Approver
	auditor  Auditor
	actor    string
	now      func() time.Time
}

// NewMemoryGate returns an empty fail-closed gate (PIN_MISSING until an
// initial pin is installed).
func NewMemoryGate() *MemoryGate {
	return &MemoryGate{now: func() time.Time { return time.Now().UTC() }}
}

// NewFileGate is the production pin store. A missing or tampered file is
// fail-closed; Evaluate never creates the file.
func NewFileGate(path string) *MemoryGate {
	return &MemoryGate{store: &FileStore{Path: path}, now: func() time.Time { return time.Now().UTC() }}
}

// SetApprover attaches the #6 HITL hook used by ResolveApproval.
func (g *MemoryGate) SetApprover(a Approver) { g.approver = a }

// SetAuditor attaches the #8 hook. Approve/deny emit who/when/oldHash/newHash.
func (g *MemoryGate) SetAuditor(a Auditor) { g.auditor = a }

// SetActor is the default "who" written on approved/denied events.
func (g *MemoryGate) SetActor(who string) { g.actor = who }

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

// ApplyApproval is apply_approval(pin_revision, approve|deny).
//
// G approve: pin becomes the pending candidate (newHash only).
// H deny: old pin is kept; hash does not advance. Timeout is mapped to
// deny by MapApproverResult before this call.
func (g *MemoryGate) ApplyApproval(pinRevision string, outcome ApprovalOutcome) GateDecision {
	return g.applyApproval(pinRevision, outcome, "deny")
}

// ResolveApproval runs Approver.RequestApproval then apply_approval.
// Timeout / deny / version mismatch → APPROVAL_DENIED, old pin kept (H).
func (g *MemoryGate) ResolveApproval(ctx context.Context) GateDecision {
	g.mu.Lock()
	if g.pending == nil {
		g.mu.Unlock()
		return Deny(ReasonInternalError, nil)
	}
	diff := *copyDiff(g.pending.Diff)
	cand := *clonePin(g.pending.Candidate)
	approver := g.approver
	g.mu.Unlock()
	if approver == nil {
		return Deny(ReasonApprovalPending, &diff)
	}
	ver, err := approver.RequestApproval(ctx, diff, cand)
	return g.applyApproval(cand.Version, MapApproverResult(ver, err, cand.Version), ApproverDecisionLabel(err))
}

func (g *MemoryGate) applyApproval(pinRevision string, outcome ApprovalOutcome, decision string) GateDecision {
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
		// Fail-closed: do not advance; leave pending so all calls stay denied (F).
		return Deny(ReasonInternalError, copyDiff(g.pending.Diff))
	}
	diff := copyDiff(g.pending.Diff)
	oldHash := ""
	oldRev := ""
	if g.pin != nil {
		oldHash = g.pin.Aggregate
		oldRev = g.pin.Version
	}
	who := g.actor
	if who == "" {
		who = "local"
	}
	when := g.now()
	if g.now == nil {
		when = time.Now().UTC()
	}

	switch outcome {
	case OutcomeApprove:
		g.pin = clonePin(g.pending.Candidate)
		g.pending = nil
		if g.store != nil {
			if err := g.store.Save(*g.pin); err != nil {
				return Deny(ReasonInternalError, diff)
			}
		}
		g.emit("approved", map[string]any{
			"who":          who,
			"when":         when.Format(time.RFC3339Nano),
			"oldHash":      oldHash,
			"newHash":      g.pin.Aggregate,
			"pin_revision": g.pin.Version,
			"pin_hash":     oldHash,
			"live_hash":    g.pin.Aggregate,
			"reason_code":  string(ReasonOK),
			"decision":     "approve",
		})
		return Allow(g.pin.Version)
	case OutcomeDeny:
		g.pending = nil
		if decision == "" || decision == "approve" {
			decision = "deny"
		}
		g.emit("denied", map[string]any{
			"who":          who,
			"when":         when.Format(time.RFC3339Nano),
			"oldHash":      oldHash,
			"newHash":      oldHash, // Chief H: hash does not advance
			"pin_revision": oldRev,
			"pin_hash":     oldHash,
			"live_hash":    diff.LiveHash,
			"reason_code":  string(ReasonApprovalDenied),
			"decision":     decision,
		})
		return Deny(ReasonApprovalDenied, diff)
	default:
		return Deny(ReasonInternalError, diff)
	}
}

func (g *MemoryGate) emit(event string, fields map[string]any) {
	if g.auditor == nil {
		return
	}
	g.auditor.Record(event, fields)
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
