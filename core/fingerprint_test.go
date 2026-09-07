package core

import (
	"context"
	"testing"
)

func githubCfg(argv []string, environ ...string) ConfigIdentity {
	return NewConfigIdentity(argv, environ)
}

func TestConfigFingerprintStableArgvAndEnvSubset(t *testing.T) {
	a := ConfigFingerprint(
		[]string{"/usr/bin/github-mcp-server", "--toolsets", "issues,repos"},
		map[string]string{"GITHUB_TOOLSETS": "repos,issues"},
	)
	b := ConfigFingerprint(
		[]string{"/usr/bin/github-mcp-server", "--toolsets", "repos,issues"},
		map[string]string{"GITHUB_TOOLSETS": "issues,repos"},
	)
	if a == "" || a != b {
		t.Fatalf("stable(--toolsets CSV) must match: %s vs %s", a, b)
	}
	inline := ConfigFingerprint(
		[]string{"github-mcp-server", "--tools=fetch,repos"},
		nil,
	)
	spaced := ConfigFingerprint(
		[]string{"github-mcp-server", "--tools", "repos,fetch"},
		nil,
	)
	if inline != spaced {
		t.Fatalf("--tools inline vs argv must match: %s vs %s", inline, spaced)
	}
}

func TestConfigFingerprintIgnoresSecretEnv(t *testing.T) {
	base := githubCfg([]string{"github-mcp-server"}, "GITHUB_TOOLSETS=repos")
	withToken := githubCfg(
		[]string{"github-mcp-server"},
		"GITHUB_TOOLSETS=repos",
		"GITHUB_PERSONAL_ACCESS_TOKEN=ghp_secret",
		"GITHUB_TOKEN=also-secret",
	)
	if base.Fingerprint() != withToken.Fingerprint() {
		t.Fatal("tokens must not enter config_fingerprint")
	}
	if _, ok := withToken.Env["GITHUB_PERSONAL_ACCESS_TOKEN"]; ok {
		t.Fatal("SubsetEnv leaked a token")
	}
}

func TestConfigFingerprintEnvInventoryChange(t *testing.T) {
	a := githubCfg([]string{"github-mcp-server"}, "GITHUB_TOOLSETS=repos")
	b := githubCfg([]string{"github-mcp-server"}, "GITHUB_TOOLSETS=repos,issues")
	if a.Fingerprint() == b.Fingerprint() {
		t.Fatal("GITHUB_TOOLSETS change must change fingerprint")
	}
	scopeA := githubCfg([]string{"github-mcp-server"}, "DEADBUGZ_CREDENTIAL_SCOPE=repo")
	scopeB := githubCfg([]string{"github-mcp-server"}, "DEADBUGZ_CREDENTIAL_SCOPE=repo,gist")
	if scopeA.Fingerprint() == scopeB.Fingerprint() {
		t.Fatal("credential-scope inventory change must change fingerprint")
	}
}

func TestPinIDHashServerFingerprintTools(t *testing.T) {
	tools := []ToolDef{{Name: "search", Description: "q"}}
	p := PinToolsWithConfig(tools, "pin-1", githubCfg([]string{"/opt/github-mcp-server", "--tools", "search"}))
	want := ComputePinID(p.ServerName, p.ConfigFingerprint, p.Aggregate)
	if p.ServerName != "github-mcp-server" || p.PinID == "" || p.PinID != want {
		t.Fatalf("pin_id=%s want %s server=%s", p.PinID, want, p.ServerName)
	}
	otherServer := ComputePinID("other", p.ConfigFingerprint, p.Aggregate)
	if otherServer == p.PinID {
		t.Fatal("server_name must be in pin_id")
	}
	otherTools := PinTools([]ToolDef{{Name: "search", Description: "changed"}}, "x")
	if ComputePinID(p.ServerName, p.ConfigFingerprint, otherTools.Aggregate) == p.PinID {
		t.Fatal("canonical_tools_hash must be in pin_id")
	}
}

func TestEvaluateConfigChangeEvenIfToolsHashMatches(t *testing.T) {
	tools := []ToolDef{{Name: "search", Description: "q"}}
	oldCfg := githubCfg([]string{"github-mcp-server", "--toolsets", "repos"})
	newCfg := githubCfg([]string{"github-mcp-server", "--toolsets", "repos,issues"})
	g := NewMemoryGate()
	g.SetConfig(newCfg)
	if err := g.InstallInitialPin(PinToolsWithConfig(tools, "pin-1", oldCfg)); err != nil {
		t.Fatal(err)
	}
	oldHash := g.CurrentPin().Aggregate
	d := g.Evaluate(context.Background(), tools)
	if d.Allowed || d.ReasonCode != ReasonConfigOrInventoryChanged {
		t.Fatalf("fingerprint change at start: %+v", d)
	}
	if d.Diff == nil || d.Diff.LiveHash != oldHash || d.Diff.PinHash != oldHash {
		t.Fatalf("tools hash still matches, diff hashes: %+v", d.Diff)
	}
	if d.Diff.Empty() != true {
		t.Fatalf("no tool add/remove/change expected: %+v", d.Diff)
	}
	if g.CurrentPin().Aggregate != oldHash {
		t.Fatal("config change must not auto-promote the pin")
	}
	if g.PendingCandidate() == nil || g.PendingCandidate().ConfigFingerprint == oldCfg.Fingerprint() {
		t.Fatal("candidate must carry the new fingerprint")
	}
}

func TestEvaluateToolsListDriftSameFingerprint(t *testing.T) {
	cfg := githubCfg([]string{"github-mcp-server", "--tools", "search"})
	g := NewMemoryGate()
	g.SetConfig(cfg)
	if err := g.InstallInitialPin(PinToolsWithConfig([]ToolDef{{Name: "search", Description: "q"}}, "pin-1", cfg)); err != nil {
		t.Fatal(err)
	}
	old := g.CurrentPin()
	d := g.Evaluate(context.Background(), []ToolDef{{Name: "search", Description: "poisoned"}})
	if d.Allowed || d.ReasonCode != ReasonToolsListDrift {
		t.Fatalf("same fingerprint + live mutate: %+v", d)
	}
	if d.Diff == nil || len(d.Diff.Changed) != 1 || d.Diff.Changed[0] != "search" {
		t.Fatalf("drift diff: %+v", d.Diff)
	}
	if g.CurrentPin().Aggregate != old.Aggregate || g.CurrentPin().PinID != old.PinID {
		t.Fatal("tools_list_drift must never auto-promote")
	}
	again := g.Evaluate(context.Background(), []ToolDef{{Name: "search", Description: "q"}})
	if again.Allowed || again.ReasonCode != ReasonApprovalPending {
		t.Fatalf("pending after drift: %+v", again)
	}
}

func TestEvaluateReorderOnlySameFingerprintAllows(t *testing.T) {
	cfg := githubCfg([]string{"github-mcp-server"})
	g := NewMemoryGate()
	g.SetConfig(cfg)
	if err := g.InstallInitialPin(PinToolsWithConfig(loadFixture(t, "benign"), "pin-1", cfg)); err != nil {
		t.Fatal(err)
	}
	d := g.Evaluate(context.Background(), loadFixture(t, "reorder"))
	if !d.Allowed || d.ReasonCode != ReasonOK {
		t.Fatalf("reorder-only under same fingerprint: %+v", d)
	}
	if d2 := g.Evaluate(context.Background(), loadFixture(t, "key_reorder")); !d2.Allowed {
		t.Fatalf("key-reorder under same fingerprint: %+v", d2)
	}
}

func TestEvaluateLegacyPinKeepsDiffNonempty(t *testing.T) {
	g := NewMemoryGate()
	g.SetConfig(githubCfg([]string{"github-mcp-server", "--tools", "search"}))
	if err := g.InstallInitialPin(PinTools([]ToolDef{{Name: "search", Description: "q"}}, "pin-1")); err != nil {
		t.Fatal(err)
	}
	// Legacy pin has no fingerprint: tools match still allows (no silent stamp).
	if d := g.Evaluate(context.Background(), []ToolDef{{Name: "search", Description: "q"}}); !d.Allowed {
		t.Fatalf("legacy pin + matching tools: %+v", d)
	}
	if g.CurrentPin().ConfigFingerprint != "" {
		t.Fatal("Evaluate must not silently stamp a fingerprint")
	}
	d := g.Evaluate(context.Background(), []ToolDef{{Name: "search", Description: "poisoned"}})
	if d.Allowed || d.ReasonCode != ReasonDiffNonempty {
		t.Fatalf("legacy tools mismatch: %+v", d)
	}
}

func TestApplyApprovalConfigChangeWritesNewFingerprint(t *testing.T) {
	tools := []ToolDef{{Name: "search", Description: "q"}}
	oldCfg := githubCfg([]string{"github-mcp-server", "--toolsets", "repos"})
	newCfg := githubCfg([]string{"github-mcp-server", "--toolsets", "issues"})
	g := NewMemoryGate()
	g.SetConfig(newCfg)
	if err := g.InstallInitialPin(PinToolsWithConfig(tools, "pin-1", oldCfg)); err != nil {
		t.Fatal(err)
	}
	d := g.Evaluate(context.Background(), tools)
	if d.ReasonCode != ReasonConfigOrInventoryChanged {
		t.Fatalf("setup: %+v", d)
	}
	cand := g.PendingCandidate()
	got := g.ApplyApproval(cand.Version, OutcomeApprove)
	if !got.Allowed {
		t.Fatalf("approve config change: %+v", got)
	}
	pin := g.CurrentPin()
	if pin.ConfigFingerprint != newCfg.Fingerprint() {
		t.Fatalf("approved fingerprint %s want %s", pin.ConfigFingerprint, newCfg.Fingerprint())
	}
	if pin.PinID != cand.PinID {
		t.Fatalf("approved pin_id %s want %s", pin.PinID, cand.PinID)
	}
}

func TestEvaluateWithoutSetConfigIgnoresStoredFingerprint(t *testing.T) {
	cfg := githubCfg([]string{"github-mcp-server"})
	g := NewMemoryGate()
	p := PinToolsWithConfig([]ToolDef{{Name: "search", Description: "q"}}, "pin-1", cfg)
	if err := g.InstallInitialPin(p); err != nil {
		t.Fatal(err)
	}
	d := g.Evaluate(context.Background(), []ToolDef{{Name: "search", Description: "q"}})
	if !d.Allowed {
		t.Fatalf("Evaluate(live) without SetConfig must keep #5 allow: %+v", d)
	}
}

func TestFileStorePersistsPinIdentity(t *testing.T) {
	path := t.TempDir() + "/pin.json"
	st := &FileStore{Path: path}
	p := PinToolsWithConfig([]ToolDef{{Name: "search", Description: "q"}}, "pin-1", githubCfg([]string{"github-mcp-server", "--tools", "search"}))
	if err := st.Save(p); err != nil {
		t.Fatal(err)
	}
	got, err := st.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.ConfigFingerprint != p.ConfigFingerprint || got.PinID != p.PinID || got.ServerName != p.ServerName {
		t.Fatalf("store identity: %+v want fp=%s id=%s", got, p.ConfigFingerprint, p.PinID)
	}
}

func TestRequiresApprovalReasonCodes(t *testing.T) {
	if !ReasonConfigOrInventoryChanged.RequiresApproval() || !ReasonToolsListDrift.RequiresApproval() || !ReasonDiffNonempty.RequiresApproval() {
		t.Fatal("mismatch codes must require approval")
	}
	if ReasonOK.RequiresApproval() || ReasonPinMissing.RequiresApproval() {
		t.Fatal("OK / PIN_MISSING are not re-approval asks")
	}
}
