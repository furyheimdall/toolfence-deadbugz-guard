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
