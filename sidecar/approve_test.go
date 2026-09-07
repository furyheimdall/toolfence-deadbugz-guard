package sidecar

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/furyheimdall/toolfence-deadbugz-guard/audit"
	"github.com/furyheimdall/toolfence-deadbugz-guard/core"
	"github.com/furyheimdall/toolfence-deadbugz-guard/internal/mockmcp"
)

func TestGateFromPinFileTamperIsEmpty(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.json")
	g := GateFromPinFile(missing)
	if d := g.Evaluate(context.Background(), mockmcp.Tools(mockmcp.ModeBenign)); d.Allowed || d.ReasonCode != core.ReasonPinMissing {
		t.Fatalf("missing: %+v", d)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("GateFromPinFile must not create a missing pin")
	}

	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	g = GateFromPinFile(bad)
	if d := g.Evaluate(context.Background(), mockmcp.Tools(mockmcp.ModeBenign)); d.Allowed || d.ReasonCode != core.ReasonPinMissing {
		t.Fatalf("tamper: %+v", d)
	}
	raw, err := os.ReadFile(bad)
	if err != nil || string(raw) != "{not-json" {
		t.Fatal("tampered pin must stay unrepaired")
	}
}

func TestInspectPinFile(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "nope.json")
	if got := InspectPinFile(missing); got != PinFileAbsent {
		t.Fatalf("absent: %v", got)
	}
	ok := filepath.Join(dir, "ok.json")
	if _, err := WritePinFile(ok, "pin-1", mockmcp.Tools(mockmcp.ModeBenign)); err != nil {
		t.Fatal(err)
	}
	if got := InspectPinFile(ok); got != PinFileOK {
		t.Fatalf("ok: %v", got)
	}
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := InspectPinFile(bad); got != PinFileTampered {
		t.Fatalf("tampered: %v", got)
	}
}

func TestWritePinFileMode0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pins", "filesystem.json")
	if _, err := WritePinFile(path, "pin-1", mockmcp.Tools(mockmcp.ModeBenign)); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != PinFileMode {
		t.Fatalf("pin mode=%o want %o", st.Mode().Perm(), PinFileMode)
	}
}

func TestNonTTYBootstrapApproveWritesPin(t *testing.T) {
	cfg := Config{PinPath: filepath.Join(t.TempDir(), "filesystem.json"), Approve: true}
	p, ok, err := ApplyNonTTYApprove(cfg, core.ReasonPinMissing, mockmcp.Tools(mockmcp.ModeBenign))
	if err != nil || !ok {
		t.Fatalf("bootstrap: ok=%v err=%v", ok, err)
	}
	if p.Aggregate == "" {
		t.Fatal("empty hash")
	}
	if InspectPinFile(cfg.PinPath) != PinFileOK {
		t.Fatal("pin not durable")
	}
}

func TestNonTTYApproveDoesNotRepairTamper(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pin.json")
	if err := os.WriteFile(path, []byte(`{"version":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{PinPath: path, Approve: true}
	_, ok, err := ApplyNonTTYApprove(cfg, core.ReasonPinMissing, mockmcp.Tools(mockmcp.ModeBenign))
	if err != nil || ok {
		t.Fatalf("tamper must not silent re-pin: ok=%v err=%v", ok, err)
	}
	raw, _ := os.ReadFile(path)
	if string(raw) == "" || raw[0] != '{' {
		t.Fatal("tampered file was replaced")
	}
	var p core.Pin
	if json.Unmarshal(raw, &p) == nil && p.Aggregate != "" {
		t.Fatal("tampered file became a pin")
	}
}

func TestNonTTYApproveFlagDoesNotRepinDrift(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pin.json")
	if _, err := WritePinFile(path, "pin-1", mockmcp.Tools(mockmcp.ModeBenign)); err != nil {
		t.Fatal(err)
	}
	cfg := Config{PinPath: path, Approve: true}
	_, ok, err := ApplyNonTTYApprove(cfg, core.ReasonDiffNonempty, mockmcp.Tools(mockmcp.ModePoison))
	if err != nil || ok {
		t.Fatalf("flag must not re-pin drift: ok=%v err=%v", ok, err)
	}
}

func TestNonTTYApproveFileConsumesOnDrift(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pin.json")
	if _, err := WritePinFile(path, "pin-1", mockmcp.Tools(mockmcp.ModeBenign)); err != nil {
		t.Fatal(err)
	}
	tok := filepath.Join(dir, "approve")
	if err := os.WriteFile(tok, []byte("approve\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{PinPath: path, ApproveFile: tok}
	_, ok, err := ApplyNonTTYApprove(cfg, core.ReasonDiffNonempty, mockmcp.Tools(mockmcp.ModePoison))
	if err != nil || !ok {
		t.Fatalf("token should re-pin drift: ok=%v err=%v", ok, err)
	}
	if _, err := os.Stat(tok); !os.IsNotExist(err) {
		t.Fatal("approve-file should be consumed")
	}
	g := GateFromPinFile(path)
	dec := g.Evaluate(context.Background(), mockmcp.Tools(mockmcp.ModePoison))
	if !dec.Allowed {
		t.Fatalf("new pin should match poison: %+v", dec)
	}
}

func TestNonTTYApproveFileAuditsWhoWhenOldNew(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pin.json")
	old, err := WritePinFile(path, "pin-1", mockmcp.Tools(mockmcp.ModeBenign))
	if err != nil {
		t.Fatal(err)
	}
	tok := filepath.Join(dir, "approve")
	if err := os.WriteFile(tok, []byte("approve\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	auditPath := filepath.Join(dir, "audit.jsonl")
	cfg := Config{PinPath: path, ApproveFile: tok, AuditPath: auditPath}
	p, ok, err := ApplyNonTTYApprove(cfg, core.ReasonDiffNonempty, mockmcp.Tools(mockmcp.ModePoison))
	if err != nil || !ok {
		t.Fatalf("token should re-pin drift: ok=%v err=%v", ok, err)
	}
	if p.Aggregate == old.Aggregate {
		t.Fatal("G: pin must advance to newHash")
	}
	rows, err := audit.Tail(auditPath, 20)
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
	if approved["who"] != "approve-file" {
		t.Fatalf("who=%v", approved["who"])
	}
	if approved["when"] == nil || approved["when"] == "" {
		t.Fatal("when missing")
	}
	if approved["oldHash"] != old.Aggregate {
		t.Fatalf("oldHash=%v want %s", approved["oldHash"], old.Aggregate)
	}
	if approved["newHash"] != p.Aggregate {
		t.Fatalf("newHash=%v want %s", approved["newHash"], p.Aggregate)
	}
}

func TestNonTTYApproveHTTPBootstrap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"decision":"approve"}`))
	}))
	t.Cleanup(srv.Close)
	cfg := Config{PinPath: filepath.Join(t.TempDir(), "pin.json"), ApproveHTTP: srv.URL}
	_, ok, err := ApplyNonTTYApprove(cfg, core.ReasonPinMissing, mockmcp.Tools(mockmcp.ModeBenign))
	if err != nil || !ok {
		t.Fatalf("http bootstrap: ok=%v err=%v", ok, err)
	}
}

func TestApproveFromPending(t *testing.T) {
	dir := t.TempDir()
	pin := filepath.Join(dir, "fetch.json")
	if err := WritePendingSnapshot(pin, core.ReasonPinMissing, mockmcp.Tools(mockmcp.ModeBenign), nil); err != nil {
		t.Fatal(err)
	}
	p, err := ApproveFromPending(pin, "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Aggregate == "" {
		t.Fatal("empty pin")
	}
	if _, err := os.Stat(PendingPath(pin)); !os.IsNotExist(err) {
		t.Fatal("pending should be removed")
	}
}

func TestApplyNonTTYApproveStampsGitHubFingerprint(t *testing.T) {
	cfg := Config{
		PinPath:    filepath.Join(t.TempDir(), "github.json"),
		Approve:    true,
		ServerName: "github",
		ServerArgv: []string{"/usr/bin/github-mcp-server", "--toolsets", "repos"},
		ServerEnv:  []string{"GITHUB_TOOLSETS=repos", "GITHUB_PERSONAL_ACCESS_TOKEN=ghp_secret"},
	}
	p, ok, err := ApplyNonTTYApprove(cfg, core.ReasonPinMissing, mockmcp.Tools(mockmcp.ModeBenign))
	if err != nil || !ok {
		t.Fatalf("bootstrap: ok=%v err=%v", ok, err)
	}
	want := processIdentity(cfg)
	if p.ConfigFingerprint == "" || p.ConfigFingerprint != want.Fingerprint() {
		t.Fatalf("fingerprint %s want %s", p.ConfigFingerprint, want.Fingerprint())
	}
	if p.PinID == "" || p.ServerName != "github" {
		t.Fatalf("identity: %+v", p)
	}
	tokenless := core.NewConfigIdentity(cfg.ServerArgv, []string{"GITHUB_TOOLSETS=repos"})
	if p.ConfigFingerprint != tokenless.Fingerprint() {
		t.Fatal("token must not enter config_fingerprint")
	}
}

func TestApproveFromServer(t *testing.T) {
	dir := t.TempDir()
	pin := filepath.Join(dir, "pin.json")
	flip := filepath.Join(dir, "flip.json")
	if err := mockmcp.WriteFlip(flip, mockmcp.FlipState{Mode: mockmcp.ModeBenign, CallGate: 3}); err != nil {
		t.Fatal(err)
	}
	mockIn, wrapIn := io.Pipe()
	wrapOut, mockOut := io.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = mockmcp.Serve(mockIn, mockOut, flip)
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p, err := ApproveFromServer(ctx, pin, func() (io.WriteCloser, io.ReadCloser, func() error, error) {
		return wrapIn, wrapOut, func() error { return nil }, nil
	})
	_ = mockIn.Close()
	_ = mockOut.Close()
	<-done
	if err != nil {
		t.Fatal(err)
	}
	if p.Aggregate == "" {
		t.Fatal("empty hash")
	}
	st, err := os.Stat(pin)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != PinFileMode {
		t.Fatalf("mode=%o", st.Mode().Perm())
	}
}

func TestExpandHomeAndDefaultPinPath(t *testing.T) {
	if got := SanitizeServerName("file system!"); got != "file_system" {
		t.Fatalf("sanitize: %q", got)
	}
	p := DefaultPinPath("filesystem")
	if filepath.Base(p) != "filesystem.json" {
		t.Fatalf("pin path %s", p)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	if got := ExpandHome("~/x"); got != filepath.Join(home, "x") {
		t.Fatalf("expand: %s", got)
	}
}

func TestParseApproveEnv(t *testing.T) {
	boot, file, u := parseApproveEnv("1")
	if !boot || file != "" || u != "" {
		t.Fatalf("1: %v %q %q", boot, file, u)
	}
	boot, file, u = parseApproveEnv("https://127.0.0.1/approve")
	if boot || file != "" || u == "" {
		t.Fatalf("url: %v %q %q", boot, file, u)
	}
	boot, file, u = parseApproveEnv("/tmp/token")
	if boot || file != "/tmp/token" {
		t.Fatalf("file: %v %q", boot, file)
	}
}
