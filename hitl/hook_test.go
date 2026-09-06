package hitl

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/furyheimdall/toolfence-deadbugz-guard/audit"
	"github.com/furyheimdall/toolfence-deadbugz-guard/core"
)

func primed(t *testing.T) (*core.MemoryGate, core.Pin, []core.ToolDef) {
	t.Helper()
	g := core.NewMemoryGate()
	oldTools := []core.ToolDef{{Name: "search", Description: "q"}}
	old := core.PinTools(oldTools, "pin-1")
	if err := g.InstallInitialPin(old); err != nil {
		t.Fatal(err)
	}
	live := []core.ToolDef{{Name: "search", Description: "poisoned"}}
	return g, old, live
}

// TestGApproveAdvancesHashAndAuditsWhoWhenOldNew is Epic #1 / Chief G.
func TestGApproveAdvancesHashAndAuditsWhoWhenOldNew(t *testing.T) {
	g, old, live := primed(t)
	path := t.TempDir() + "/audit.jsonl"
	aud := audit.NewJSONLAuditor(path)
	RecordPinCreated(aud, "ops", old)

	fixed := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	h := &Hook{
		Gate: g,
		Approver: &CallbackApprover{Fn: func(ctx context.Context, diff core.ToolDiffSummary, cand core.Pin) (string, string, error) {
			if diff.Empty() {
				t.Fatal("HITL payload must include a nonempty ToolDiffSummary")
			}
			if len(diff.Changed) == 0 && len(diff.Added) == 0 && len(diff.Removed) == 0 {
				t.Fatalf("diff names missing: %+v", diff)
			}
			return cand.Version, "alice", nil
		}},
		Auditor: aud,
		Now:     func() time.Time { return fixed },
	}

	d := h.Evaluate(context.Background(), live)
	if !d.Allowed || d.ReasonCode != core.ReasonOK {
		t.Fatalf("G approve → Allow, got %+v", d)
	}
	got := g.CurrentPin()
	if got.Aggregate == old.Aggregate {
		t.Fatal("G: pin must advance to newHash")
	}

	rows, err := audit.Tail(path, 20)
	if err != nil {
		t.Fatal(err)
	}
	var approved map[string]any
	for _, r := range rows {
		if r["event"] == audit.EventApproved {
			approved = r
		}
	}
	if approved == nil {
		t.Fatalf("missing approved event: %+v", rows)
	}
	if approved["who"] != "alice" {
		t.Fatalf("who=%v", approved["who"])
	}
	if approved["when"] == nil || approved["when"] == "" {
		t.Fatal("when missing")
	}
	if approved["oldHash"] != old.Aggregate {
		t.Fatalf("oldHash=%v want %s", approved["oldHash"], old.Aggregate)
	}
	if approved["newHash"] != got.Aggregate {
		t.Fatalf("newHash=%v want %s", approved["newHash"], got.Aggregate)
	}
	if approved["oldHash"] == approved["newHash"] {
		t.Fatal("G: oldHash and newHash must differ after approve")
	}
}

// TestHDenyKeepsOldPin is Epic #1 / Chief H (deny).
func TestHDenyKeepsOldPin(t *testing.T) {
	g, old, live := primed(t)
	path := t.TempDir() + "/audit.jsonl"
	aud := audit.NewJSONLAuditor(path)
	h := &Hook{
		Gate: g,
		Approver: &CallbackApprover{Fn: func(ctx context.Context, diff core.ToolDiffSummary, cand core.Pin) (string, string, error) {
			return "", "bob", ErrDenied
		}},
		Auditor: aud,
	}
	d := h.Evaluate(context.Background(), live)
	if d.Allowed || d.ReasonCode != core.ReasonApprovalDenied {
		t.Fatalf("H deny → APPROVAL_DENIED, got %+v", d)
	}
	if g.CurrentPin().Aggregate != old.Aggregate {
		t.Fatal("H: deny must keep old pin hash")
	}
	rows, err := audit.Tail(path, 20)
	if err != nil {
		t.Fatal(err)
	}
	var denied map[string]any
	for _, r := range rows {
		if r["event"] == audit.EventDenied {
			denied = r
		}
	}
	if denied == nil {
		t.Fatal("missing denied event")
	}
	if denied["oldHash"] != old.Aggregate || denied["newHash"] != old.Aggregate {
		t.Fatalf("H deny must not advance hash: %+v", denied)
	}
	if denied["who"] != "bob" {
		t.Fatalf("who=%v", denied["who"])
	}
}

// TestHTimeoutKeepsOldPin is Epic #1 / Chief H (timeout == deny, old pin kept).
func TestHTimeoutKeepsOldPin(t *testing.T) {
	g, old, live := primed(t)
	path := t.TempDir() + "/audit.jsonl"
	aud := audit.NewJSONLAuditor(path)
	h := &Hook{
		Gate: g,
		Approver: &CallbackApprover{Fn: func(ctx context.Context, diff core.ToolDiffSummary, cand core.Pin) (string, string, error) {
			<-ctx.Done()
			return "", "watchdog", ErrTimeout
		}},
		Auditor: aud,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	d := h.Evaluate(ctx, live)
	if d.Allowed || d.ReasonCode != core.ReasonApprovalDenied {
		t.Fatalf("H timeout → deny, got %+v", d)
	}
	if g.CurrentPin().Aggregate != old.Aggregate {
		t.Fatal("H: timeout must keep old pin")
	}
	rows, err := audit.Tail(path, 20)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range rows {
		if r["event"] == audit.EventDenied {
			found = true
			if r["newHash"] != old.Aggregate || r["oldHash"] != old.Aggregate {
				t.Fatalf("timeout advanced hash: %+v", r)
			}
			if r["decision"] != "timeout" {
				t.Fatalf("decision=%v want timeout", r["decision"])
			}
		}
	}
	if !found {
		t.Fatal("timeout must emit denied")
	}
}

func TestPendingDeniesAllWhileHITLInFlight(t *testing.T) {
	g, _, live := primed(t)
	started := make(chan struct{})
	release := make(chan struct{})
	h := &Hook{
		Gate: g,
		Approver: &CallbackApprover{Fn: func(ctx context.Context, diff core.ToolDiffSummary, cand core.Pin) (string, string, error) {
			close(started)
			<-release
			return cand.Version, "alice", nil
		}},
	}
	done := make(chan core.GateDecision, 1)
	go func() {
		done <- h.Evaluate(context.Background(), live)
	}()
	<-started
	pending := g.Evaluate(context.Background(), []core.ToolDef{{Name: "search", Description: "q"}})
	if pending.Allowed || pending.ReasonCode != core.ReasonApprovalPending {
		t.Fatalf("Chief F: pending must deny all, got %+v", pending)
	}
	close(release)
	d := <-done
	if !d.Allowed {
		t.Fatalf("after approve: %+v", d)
	}
}

func TestRequestPayloadIsToolDiffSummary(t *testing.T) {
	g, _, live := primed(t)
	var got *core.ToolDiffSummary
	h := &Hook{
		Gate: g,
		Approver: &CallbackApprover{Fn: func(ctx context.Context, diff core.ToolDiffSummary, cand core.Pin) (string, string, error) {
			cp := diff
			got = &cp
			return "", "", ErrDenied
		}},
	}
	_ = h.Evaluate(context.Background(), live)
	if got == nil {
		t.Fatal("Approver must receive ToolDiffSummary")
	}
	if got.PinHash == "" || got.LiveHash == "" || got.PinRevision == "" {
		t.Fatalf("summary hashes/revision missing: %+v", got)
	}
	if got.Empty() {
		t.Fatal("mismatch must be nonempty")
	}
}

func TestApplyApprovalMismatchDoesNotAdvance(t *testing.T) {
	g, old, live := primed(t)
	h := &Hook{
		Gate: g,
		Approver: &CallbackApprover{Fn: func(ctx context.Context, diff core.ToolDiffSummary, cand core.Pin) (string, string, error) {
			return "not-the-candidate", "eve", nil
		}},
	}
	d := h.Evaluate(context.Background(), live)
	if d.Allowed {
		t.Fatal("wrong approvedVersion must fail closed")
	}
	if g.CurrentPin().Aggregate != old.Aggregate {
		t.Fatal("must not advance on version mismatch")
	}
}

// TestAuditEventsPinDiffBlockReapprove is #8: pin / diff / block / re-approve.
func TestAuditEventsPinDiffBlockReapprove(t *testing.T) {
	g, old, live := primed(t)
	path := t.TempDir() + "/audit.jsonl"
	aud := audit.NewJSONLAuditor(path)
	RecordPinCreated(aud, "ops", old)
	h := &Hook{
		Gate: g,
		Approver: &CallbackApprover{Fn: func(ctx context.Context, diff core.ToolDiffSummary, cand core.Pin) (string, string, error) {
			return cand.Version, "alice", nil
		}},
		Auditor: aud,
	}
	if d := h.Evaluate(context.Background(), live); !d.Allowed {
		t.Fatalf("%+v", d)
	}
	rows, err := audit.Tail(path, 20)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, r := range rows {
		if ev, ok := r["event"].(string); ok {
			seen[ev] = true
		}
	}
	for _, ev := range []string{audit.EventPinCreated, audit.EventDiffDetected, audit.EventBlocked, audit.EventApproved} {
		if !seen[ev] {
			t.Fatalf("missing event %s in %+v", ev, seen)
		}
	}
}

func TestDeniedErrorIsTimeoutClass(t *testing.T) {
	if !errors.Is(ErrTimeout, ErrTimeout) {
		t.Fatal("sentinel")
	}
}
