package sidecar

import (
	"context"
	"testing"

	"github.com/furyheimdall/toolfence-deadbugz-guard/core"
	"github.com/furyheimdall/toolfence-deadbugz-guard/internal/mockmcp"
)

func TestHashReorderInvariant(t *testing.T) {
	a := core.PinTools(mockmcp.Tools(mockmcp.ModeBenign), "v")
	b := core.PinTools(mockmcp.Tools(mockmcp.ModeReorder), "v")
	if a.Aggregate != b.Aggregate {
		t.Fatalf("reorder must not change pin hash:\nbenign  %s\nreorder %s", a.Aggregate, b.Aggregate)
	}
}

func TestDiffReorderEmptyPoisonChanged(t *testing.T) {
	ctx := context.Background()

	g := core.NewMemoryGate()
	if err := g.InstallInitialPin(core.PinTools(mockmcp.Tools(mockmcp.ModeBenign), "smoke-1")); err != nil {
		t.Fatal(err)
	}
	re := g.Evaluate(ctx, mockmcp.Tools(mockmcp.ModeReorder))
	if !re.Allowed {
		t.Fatalf("reorder should allow: %+v", re)
	}

	g2 := core.NewMemoryGate()
	if err := g2.InstallInitialPin(core.PinTools(mockmcp.Tools(mockmcp.ModeBenign), "smoke-1")); err != nil {
		t.Fatal(err)
	}
	po := g2.Evaluate(ctx, mockmcp.Tools(mockmcp.ModePoison))
	if po.Allowed || po.ReasonCode != core.ReasonDiffNonempty {
		t.Fatalf("poison should deny DIFF_NONEMPTY: %+v", po)
	}
	if po.Diff == nil || len(po.Diff.Changed) == 0 {
		t.Fatalf("poison should change a tool: %+v", po)
	}
}
