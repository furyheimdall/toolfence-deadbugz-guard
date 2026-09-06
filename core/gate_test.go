package core

import (
	"context"
	"testing"
)

func TestPinToolsReorderOnlySameAggregate(t *testing.T) {
	a := []ToolDef{
		{Name: "b", Description: "B", InputSchema: []byte(`{"type":"object"}`)},
		{Name: "a", Description: "A", InputSchema: []byte(`{"type":"object"}`)},
	}
	b := []ToolDef{
		{Name: "a", Description: "A", InputSchema: []byte(`{"type":"object"}`)},
		{Name: "b", Description: "B", InputSchema: []byte(`{"type":"object"}`)},
	}
	if PinTools(a, "1").Aggregate != PinTools(b, "1").Aggregate {
		t.Fatal("reorder-only must not change aggregate hash")
	}
}

func TestEvaluatePendingDeniesAll(t *testing.T) {
	g := NewMemoryGate()
	old := PinTools([]ToolDef{{Name: "search", Description: "q"}}, "pin-1")
	if err := g.InstallInitialPin(old); err != nil {
		t.Fatal(err)
	}
	d := g.Evaluate(context.Background(), []ToolDef{
		{Name: "search", Description: "q"},
		{Name: "evil", Description: "x"},
	})
	if d.Allowed || d.ReasonCode != ReasonDiffNonempty {
		t.Fatalf("first mismatch: %+v", d)
	}
	// Same live set, or even the original set — still pending → deny all.
	again := g.Evaluate(context.Background(), []ToolDef{{Name: "search", Description: "q"}})
	if again.Allowed || again.ReasonCode != ReasonApprovalPending {
		t.Fatalf("pending must deny all, got %+v", again)
	}
}

func TestApplyApprovalApproveAdvancesHash(t *testing.T) {
	g := primedMismatch(t)
	old := g.CurrentPin().Aggregate
	cand := g.PendingCandidate()
	d := g.ApplyApproval(cand.Version, OutcomeApprove)
	if !d.Allowed || d.ReasonCode != ReasonOK {
		t.Fatalf("approve: %+v", d)
	}
	got := g.CurrentPin()
	if got.Aggregate == old {
		t.Fatal("approve must advance hash")
	}
	if got.Aggregate != cand.Aggregate {
		t.Fatalf("newHash=%s want %s", got.Aggregate, cand.Aggregate)
	}
}

func TestApplyApprovalDenyKeepsOldHash(t *testing.T) {
	g := primedMismatch(t)
	old := g.CurrentPin().Aggregate
	cand := g.PendingCandidate()
	d := g.ApplyApproval(cand.Version, OutcomeDeny)
	if d.Allowed || d.ReasonCode != ReasonApprovalDenied {
		t.Fatalf("deny: %+v", d)
	}
	if g.CurrentPin().Aggregate != old {
		t.Fatal("deny must keep old pin hash")
	}
	if g.PendingCandidate() != nil {
		t.Fatal("deny should clear pending so HITL can be retried")
	}
}

func TestG_ApproveAdvancesHashAndAuditsWhoWhenOldNew(t *testing.T) {
	rec := &recordingAuditor{}
	g := primedMismatch(t)
	g.SetAuditor(rec)
	g.SetActor("alice")
	old := g.CurrentPin()
	cand := g.PendingCandidate()
	d := g.ApplyApproval(cand.Version, OutcomeApprove)
	if !d.Allowed || d.ReasonCode != ReasonOK {
		t.Fatalf("G: %+v", d)
	}
	got := g.CurrentPin()
	if got.Aggregate == old.Aggregate || got.Aggregate != cand.Aggregate {
		t.Fatalf("G: hash did not advance to newHash old=%s new=%s cand=%s", old.Aggregate, got.Aggregate, cand.Aggregate)
	}
	ev := rec.last("approved")
	if ev == nil {
		t.Fatal("G: missing approved audit event")
	}
	if ev["who"] != "alice" || ev["oldHash"] != old.Aggregate || ev["newHash"] != got.Aggregate {
		t.Fatalf("G audit: %+v", ev)
	}
	if ev["when"] == nil || ev["when"] == "" {
		t.Fatal("G: when missing")
	}
	if ev["oldHash"] == ev["newHash"] {
		t.Fatal("G: oldHash and newHash must differ")
	}
}

func TestH_DenyKeepsOldPinAndFailClosed(t *testing.T) {
	rec := &recordingAuditor{}
	g := primedMismatch(t)
	g.SetAuditor(rec)
	g.SetActor("bob")
	old := g.CurrentPin().Aggregate
	live := []ToolDef{{Name: "search", Description: "poisoned"}}
	d := g.ApplyApproval(g.PendingCandidate().Version, OutcomeDeny)
	if d.Allowed || d.ReasonCode != ReasonApprovalDenied {
		t.Fatalf("H deny: %+v", d)
	}
	if g.CurrentPin().Aggregate != old {
		t.Fatal("H: deny must keep old pin")
	}
	ev := rec.last("denied")
	if ev == nil || ev["oldHash"] != old || ev["newHash"] != old || ev["who"] != "bob" {
		t.Fatalf("H deny audit: %+v", ev)
	}
	again := g.Evaluate(context.Background(), live)
	if again.Allowed {
		t.Fatal("H: after deny, poisoned list must stay fail-closed")
	}
}

func TestH_TimeoutMappedToDenyKeepsOldPin(t *testing.T) {
	rec := &recordingAuditor{}
	g := primedMismatch(t)
	g.SetAuditor(rec)
	g.SetActor("watchdog")
	g.SetApprover(ApproverFunc(func(ctx context.Context, diff ToolDiffSummary, cand Pin) (string, error) {
		return "", ErrApprovalTimeout
	}))
	old := g.CurrentPin().Aggregate
	d := g.ResolveApproval(context.Background())
	if d.Allowed || d.ReasonCode != ReasonApprovalDenied {
		t.Fatalf("H timeout: %+v", d)
	}
	if g.CurrentPin().Aggregate != old {
		t.Fatal("H: timeout must not advance hash")
	}
	ev := rec.last("denied")
	if ev == nil || ev["decision"] != "timeout" || ev["newHash"] != old {
		t.Fatalf("H timeout audit: %+v", ev)
	}
	if MapApproverResult("", ErrApprovalTimeout, "pin-2") != OutcomeDeny {
		t.Fatal("timeout must map to deny")
	}
}

type recordingAuditor struct {
	events []map[string]any
}

func (r *recordingAuditor) Record(event string, fields map[string]any) {
	row := map[string]any{"event": event}
	for k, v := range fields {
		row[k] = v
	}
	r.events = append(r.events, row)
}

func (r *recordingAuditor) last(event string) map[string]any {
	for i := len(r.events) - 1; i >= 0; i-- {
		if r.events[i]["event"] == event {
			return r.events[i]
		}
	}
	return nil
}

// ApproverFunc adapts a function to Approver.
type ApproverFunc func(ctx context.Context, diff ToolDiffSummary, candidate Pin) (string, error)

func (f ApproverFunc) RequestApproval(ctx context.Context, diff ToolDiffSummary, candidate Pin) (string, error) {
	return f(ctx, diff, candidate)
}

func primedMismatch(t *testing.T) *MemoryGate {
	t.Helper()
	g := NewMemoryGate()
	if err := g.InstallInitialPin(PinTools([]ToolDef{{Name: "search", Description: "q"}}, "pin-1")); err != nil {
		t.Fatal(err)
	}
	d := g.Evaluate(context.Background(), []ToolDef{
		{Name: "search", Description: "poisoned"},
	})
	if d.ReasonCode != ReasonDiffNonempty {
		t.Fatalf("setup: %+v", d)
	}
	return g
}
