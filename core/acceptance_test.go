package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func loadFixture(t *testing.T, name string) []ToolDef {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	tools, err := ParseToolsListBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	return tools
}

func seeded(t *testing.T, tools []ToolDef) *MemoryGate {
	t.Helper()
	g := NewMemoryGate()
	if err := g.InstallInitialPin(PinTools(tools, "pin-1")); err != nil {
		t.Fatal(err)
	}
	return g
}

func TestA_SameNameChangedDescriptionOrSchema(t *testing.T) {
	g := seeded(t, loadFixture(t, "benign"))
	d := g.Evaluate(context.Background(), loadFixture(t, "poison"))
	if d.Allowed || d.ReasonCode != ReasonDiffNonempty {
		t.Fatalf("A poison: %+v", d)
	}
	if d.Diff == nil || len(d.Diff.Changed) != 1 || d.Diff.Changed[0] != "fetch" {
		t.Fatalf("A changed names: %+v", d.Diff)
	}
	g2 := seeded(t, loadFixture(t, "benign"))
	s := g2.Evaluate(context.Background(), loadFixture(t, "schema_change"))
	if s.Allowed || s.Diff == nil || len(s.Diff.Changed) != 1 || s.Diff.Changed[0] != "fetch" {
		t.Fatalf("A schema: %+v", s)
	}
}

func TestB_AddedToolsFailClosed(t *testing.T) {
	g := seeded(t, loadFixture(t, "benign"))
	d := g.Evaluate(context.Background(), loadFixture(t, "add"))
	if d.Allowed || d.ReasonCode != ReasonDiffNonempty {
		t.Fatalf("B: %+v", d)
	}
	if d.Diff == nil || len(d.Diff.Added) != 1 || d.Diff.Added[0] != "shell_exec" {
		t.Fatalf("B added names: %+v", d.Diff)
	}
}

func TestC_RemovedToolsFailClosed(t *testing.T) {
	g := seeded(t, loadFixture(t, "benign"))
	d := g.Evaluate(context.Background(), loadFixture(t, "remove"))
	if d.Allowed || d.ReasonCode != ReasonDiffNonempty {
		t.Fatalf("C: %+v", d)
	}
	if d.Diff == nil || len(d.Diff.Removed) != 1 || d.Diff.Removed[0] != "read_file" {
		t.Fatalf("C removed names: %+v", d.Diff)
	}
}

func TestD_ReorderOnlyAllowed(t *testing.T) {
	g := seeded(t, loadFixture(t, "benign"))
	if d := g.Evaluate(context.Background(), loadFixture(t, "reorder")); !d.Allowed {
		t.Fatalf("D reorder: %+v", d)
	}
	if d := g.Evaluate(context.Background(), loadFixture(t, "key_reorder")); !d.Allowed {
		t.Fatalf("D key-reorder: %+v", d)
	}
}

func TestE_DeadbugzCallGate3PoisonBlocked(t *testing.T) {
	g := seeded(t, loadFixture(t, "benign"))
	benign := loadFixture(t, "benign")
	for i := 0; i < 3; i++ {
		d := g.AuthorizeCall(context.Background(), "fetch", benign)
		if !d.Allowed {
			t.Fatalf("E call %d: %+v", i+1, d)
		}
	}
	d := g.AuthorizeCall(context.Background(), "fetch", loadFixture(t, "poison"))
	if d.Allowed {
		t.Fatal("E: after gate, poison must be blocked")
	}
}

func TestF_PendingDeniesAllToolCalls(t *testing.T) {
	g := seeded(t, loadFixture(t, "benign"))
	if d := g.Evaluate(context.Background(), loadFixture(t, "poison")); d.Allowed {
		t.Fatal(d)
	}
	for _, name := range []string{"fetch", "read_file", "unrelated"} {
		d := g.AuthorizeCall(context.Background(), name, loadFixture(t, "poison"))
		if d.Allowed || d.ReasonCode != ReasonApprovalPending {
			t.Fatalf("F %s: %+v", name, d)
		}
	}
}

func TestI_PinDeleteOrTamperNoSilentRepin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pin.json")
	g := NewFileGate(path)
	benign := loadFixture(t, "benign")
	if err := g.InstallInitialPin(PinTools(benign, "pin-1")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	d := g.Evaluate(context.Background(), benign)
	if d.Allowed || d.ReasonCode != ReasonPinMissing {
		t.Fatalf("deleted pin: %+v", d)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("I: silent re-pin after delete is forbidden")
	}

	if err := g.InstallInitialPin(PinTools(benign, "pin-1")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not-json"), 0o644); err != nil {
		t.Fatal(err)
	}
	d = g.Evaluate(context.Background(), benign)
	if d.Allowed || d.ReasonCode != ReasonPinMissing {
		t.Fatalf("tampered pin: %+v", d)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "{not-json" {
		t.Fatal("I: evaluate must not repair a tampered pin")
	}
}

func TestJ_InFlightPoisonDeniesNewCalls(t *testing.T) {
	g := seeded(t, loadFixture(t, "benign"))
	if d := g.AuthorizeCall(context.Background(), "fetch", loadFixture(t, "benign")); !d.Allowed {
		t.Fatalf("pre-poison: %+v", d)
	}
	if d := g.AuthorizeCall(context.Background(), "read_file", loadFixture(t, "poison")); d.Allowed {
		t.Fatal("J: in-flight poison must deny new calls")
	}
}

func TestGH_ApproveAdvancesDenyKeepsOldPin(t *testing.T) {
	g := seeded(t, loadFixture(t, "benign"))
	old := g.CurrentPin().Aggregate
	d := g.Evaluate(context.Background(), loadFixture(t, "poison"))
	if d.Allowed || d.Diff == nil {
		t.Fatal(d)
	}
	wrong := g.ApplyApproval("not-the-revision", OutcomeApprove)
	if wrong.Allowed {
		t.Fatal("wrong revision must not clear the gate")
	}
	if g.CurrentPin().Aggregate != old {
		t.Fatal("wrong-revision approve mutated the pin")
	}
	denied := g.ApplyApproval(g.PendingCandidate().Version, OutcomeDeny)
	if denied.Allowed || denied.ReasonCode != ReasonApprovalDenied {
		t.Fatalf("deny: %+v", denied)
	}
	if g.CurrentPin().Aggregate != old {
		t.Fatal("H: deny must keep old pin")
	}
	again := g.Evaluate(context.Background(), loadFixture(t, "add"))
	if again.Allowed || again.Diff == nil {
		t.Fatal(again)
	}
	allowed := g.ApplyApproval(g.PendingCandidate().Version, OutcomeApprove)
	if !allowed.Allowed || allowed.ReasonCode != ReasonOK {
		t.Fatalf("approve: %+v", allowed)
	}
	if g.CurrentPin().Aggregate == old {
		t.Fatal("G: approve must advance to newHash")
	}
}

func TestEvaluateDoesNotInstallPin(t *testing.T) {
	g := NewMemoryGate()
	d := g.Evaluate(context.Background(), loadFixture(t, "benign"))
	if d.Allowed || d.ReasonCode != ReasonPinMissing {
		t.Fatalf("%+v", d)
	}
	if g.CurrentPin() != nil {
		t.Fatal("evaluate must not InstallInitialPin")
	}
}

func TestHashStableAndCanonical(t *testing.T) {
	a := PinTools(loadFixture(t, "benign"), "1")
	b := PinTools(loadFixture(t, "benign"), "1")
	if a.Aggregate != b.Aggregate || !validSHA256Hex(a.Aggregate) {
		t.Fatalf("unstable or non-sha256: %s vs %s", a.Aggregate, b.Aggregate)
	}
	if PinTools(loadFixture(t, "reorder"), "1").Aggregate != a.Aggregate {
		t.Fatal("reorder changed aggregate")
	}
	if PinTools(loadFixture(t, "key_reorder"), "1").Aggregate != a.Aggregate {
		t.Fatal("key reorder changed aggregate")
	}
	if PinTools(loadFixture(t, "poison"), "1").Aggregate == a.Aggregate {
		t.Fatal("poison must change hash")
	}
}

func TestParseToolsListBareArray(t *testing.T) {
	tools, err := ParseToolsListBytes([]byte(`[{"name":"a","description":"d"}]`))
	if err != nil || len(tools) != 1 || tools[0].Name != "a" {
		t.Fatalf("%+v %v", tools, err)
	}
}

func TestFileStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pin.json")
	st := &FileStore{Path: path}
	if _, err := st.Load(); !errors.Is(err, errPinMissing) {
		t.Fatalf("empty: %v", err)
	}
	p := PinTools(loadFixture(t, "benign"), "pin-1")
	if err := st.Save(p); err != nil {
		t.Fatal(err)
	}
	got, err := st.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Aggregate != p.Aggregate {
		t.Fatalf("%s vs %s", got.Aggregate, p.Aggregate)
	}
	if err := os.WriteFile(path, []byte("["), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Load(); err == nil {
		t.Fatal("corrupt pin should error")
	}
}
