package sidecar

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/furyheimdall/toolfence-deadbugz-guard/core"
	"github.com/furyheimdall/toolfence-deadbugz-guard/internal/mockmcp"
	"github.com/furyheimdall/toolfence-deadbugz-guard/sidecar/mcpio"
)

// Official GitHub MCP attach shapes (#23 / #21). Downstream listing is the
// mock fixture so CI does not need GITHUB_PERSONAL_ACCESS_TOKEN.

func TestWrapGitHubToolsetsArgvChangeIsConfigNotDrift(t *testing.T) {
	pinCfg := githubOfficialCfg([]string{"/usr/bin/github-mcp-server", "--toolsets", "repos"}, "GITHUB_TOOLSETS=repos")
	liveCfg := githubOfficialCfg([]string{"/usr/bin/github-mcp-server", "--toolsets", "repos,issues"}, "GITHUB_TOOLSETS=repos,issues")
	cli, _, stop := startGitHubWrap(t, githubWrapOpts{
		pinCfg:   pinCfg,
		liveCfg:  liveCfg,
		pinTools: mockmcp.Tools(mockmcp.ModeBenign),
		mode:     mockmcp.ModeBenign,
	})
	defer stop()

	res := mustRPC(t, cli, 1, "tools/list", map[string]any{})
	assertReason(t, res, "config_or_inventory_changed", "intentional --toolsets change")
	diff := reasonDiff(t, res)
	if !diffEmpty(diff) {
		t.Fatalf("tools hash still matches; names should be empty: %v", diff)
	}
	if rpcErrorReason(t, res.Error) == "tools_list_drift" {
		t.Fatal("toolsets change must not be silent tools_list_drift")
	}
}

func TestWrapGitHubToolsFlagChangeIsConfigNotDrift(t *testing.T) {
	pinCfg := githubOfficialCfg([]string{"github-mcp-server", "--tools", "repos"})
	liveCfg := githubOfficialCfg([]string{"github-mcp-server", "--tools", "repos,issues"})
	cli, _, stop := startGitHubWrap(t, githubWrapOpts{
		pinCfg:   pinCfg,
		liveCfg:  liveCfg,
		pinTools: mockmcp.Tools(mockmcp.ModeBenign),
		mode:     mockmcp.ModeBenign,
	})
	defer stop()

	res := mustRPC(t, cli, 1, "tools/list", map[string]any{})
	assertReason(t, res, "config_or_inventory_changed", "--tools inventory change")
}

func TestWrapGitHubToolsetsEnvChangeIsConfigNotDrift(t *testing.T) {
	argv := []string{"/opt/github-mcp-server"}
	pinCfg := githubOfficialCfg(argv, "GITHUB_TOOLSETS=repos")
	liveCfg := githubOfficialCfg(argv, "GITHUB_TOOLSETS=repos,issues")
	cli, _, stop := startGitHubWrap(t, githubWrapOpts{
		pinCfg:   pinCfg,
		liveCfg:  liveCfg,
		pinTools: mockmcp.Tools(mockmcp.ModeBenign),
		mode:     mockmcp.ModeBenign,
	})
	defer stop()

	res := mustRPC(t, cli, 1, "tools/list", map[string]any{})
	assertReason(t, res, "config_or_inventory_changed", "GITHUB_TOOLSETS env change")
}

func TestWrapGitHubToolsetsChangeWithExpandedInventoryIsConfigNotDrift(t *testing.T) {
	// Official server really adds tools when toolsets grow. Fingerprint
	// still wins: expected re-approval, not silent tools_list_drift.
	pinCfg := githubOfficialCfg([]string{"github-mcp-server", "--toolsets", "repos"})
	liveCfg := githubOfficialCfg([]string{"github-mcp-server", "--toolsets", "repos,issues"})
	cli, _, stop := startGitHubWrap(t, githubWrapOpts{
		pinCfg:   pinCfg,
		liveCfg:  liveCfg,
		pinTools: mockmcp.Tools(mockmcp.ModeBenign),
		mode:     mockmcp.ModeAdd,
	})
	defer stop()

	res := mustRPC(t, cli, 1, "tools/list", map[string]any{})
	assertReason(t, res, "config_or_inventory_changed", "toolsets + inventory expansion")
}

func TestWrapGitHubSameFingerprintSilentMutateIsToolsListDrift(t *testing.T) {
	cfg := githubOfficialCfg([]string{"github-mcp-server", "--toolsets", "repos"}, "GITHUB_TOOLSETS=repos")
	cli, _, stop := startGitHubWrap(t, githubWrapOpts{
		pinCfg:   cfg,
		liveCfg:  cfg,
		pinTools: mockmcp.Tools(mockmcp.ModeBenign),
		mode:     mockmcp.ModePoison,
	})
	defer stop()

	res := mustRPC(t, cli, 1, "tools/list", map[string]any{})
	assertReason(t, res, "tools_list_drift", "same fingerprint + live mutate")
}

func TestWrapGitHubToolsetsCSVReorderAllows(t *testing.T) {
	pinCfg := githubOfficialCfg([]string{"github-mcp-server", "--toolsets", "issues,repos"}, "GITHUB_TOOLSETS=repos,issues")
	liveCfg := githubOfficialCfg([]string{"github-mcp-server", "--toolsets", "repos,issues"}, "GITHUB_TOOLSETS=issues,repos")
	cli, _, stop := startGitHubWrap(t, githubWrapOpts{
		pinCfg:   pinCfg,
		liveCfg:  liveCfg,
		pinTools: mockmcp.Tools(mockmcp.ModeBenign),
		mode:     mockmcp.ModeBenign,
	})
	defer stop()

	res := mustRPC(t, cli, 1, "tools/list", map[string]any{})
	if res.Error != nil {
		t.Fatalf("CSV reorder under same fingerprint must allow: %s", string(res.Error))
	}
}

func TestWrapGitHubApproveFileRepinsConfigChange(t *testing.T) {
	dir := t.TempDir()
	pinPath := filepath.Join(dir, "github.json")
	tok := filepath.Join(dir, "approve")
	if err := os.WriteFile(tok, []byte("approve\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldCfg := githubOfficialCfg([]string{"github-mcp-server", "--toolsets", "repos"})
	newCfg := githubOfficialCfg([]string{"github-mcp-server", "--toolsets", "issues"})
	cli, _, stop := startGitHubWrap(t, githubWrapOpts{
		pinCfg:      oldCfg,
		liveCfg:     newCfg,
		pinTools:    mockmcp.Tools(mockmcp.ModeBenign),
		mode:        mockmcp.ModeBenign,
		approveFile: tok,
		pinPath:     pinPath,
	})
	defer stop()

	res := mustRPC(t, cli, 1, "tools/list", map[string]any{})
	if res.Error != nil {
		t.Fatalf("approve-file should re-pin config change: %s", string(res.Error))
	}
	got, err := os.ReadFile(pinPath)
	if err != nil {
		t.Fatal(err)
	}
	var pin core.Pin
	if err := json.Unmarshal(got, &pin); err != nil {
		t.Fatal(err)
	}
	if pin.ConfigFingerprint != newCfg.Fingerprint() {
		t.Fatalf("approved fingerprint %s want %s", pin.ConfigFingerprint, newCfg.Fingerprint())
	}
	if pin.PinID == "" {
		t.Fatal("approved pin_id missing")
	}
}

func githubOfficialCfg(argv []string, environ ...string) core.ConfigIdentity {
	ident := core.NewConfigIdentity(argv, environ)
	if ident.ServerName == "" {
		ident.ServerName = "github"
	}
	return ident
}

type githubWrapOpts struct {
	pinCfg      core.ConfigIdentity
	liveCfg     core.ConfigIdentity
	pinTools    []core.ToolDef
	mode        string
	approveFile string
	pinPath     string
}

func startGitHubWrap(t *testing.T, opt githubWrapOpts) (*rpcClient, string, func()) {
	t.Helper()
	dir := t.TempDir()
	flip := filepath.Join(dir, "flip.json")
	atomicFlip(t, flip, opt.mode)
	pinPath := opt.pinPath
	if pinPath == "" {
		pinPath = filepath.Join(dir, "github.json")
	}
	if _, err := WritePinFileWithConfig(pinPath, "pin-1", opt.pinTools, opt.pinCfg); err != nil {
		t.Fatal(err)
	}

	mockIn, wrapServerIn := io.Pipe()
	wrapServerOut, mockOut := io.Pipe()
	guardStdin, cliToGuard := io.Pipe()
	cliFromGuard, guardStdout := io.Pipe()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = mockmcp.Serve(mockIn, mockOut, flip)
	}()
	go func() {
		_ = Run(ctx, Config{
			PinPath:      pinPath,
			AuditPath:    filepath.Join(dir, "audit.jsonl"),
			HITLEndpoint: "http://127.0.0.1:9/hitl",
			CallGate:     3,
			ServerName:   "github",
			ServerArgv:   opt.liveCfg.Argv,
			ServerEnv:    envMapToEnviron(opt.liveCfg.Env),
			ApproveFile:  opt.approveFile,
		}, guardStdin, guardStdout, func() (io.WriteCloser, io.ReadCloser, func() error, error) {
			return wrapServerIn, wrapServerOut, func() error { return nil }, nil
		}, nil)
	}()

	cli := &rpcClient{in: cliToGuard, out: bufio.NewReader(cliFromGuard)}
	init := mustRPC(t, cli, 0, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "github-toolsets", "version": "0"},
	})
	if init.Error != nil {
		t.Fatalf("initialize: %s", string(init.Error))
	}

	return cli, pinPath, func() {
		cancel()
		_ = cliToGuard.Close()
		_ = guardStdin.Close()
		_ = mockIn.Close()
		_ = mockOut.Close()
		_ = wrapServerIn.Close()
		_ = wrapServerOut.Close()
		_ = guardStdout.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}
}

func envMapToEnviron(env map[string]string) []string {
	if env == nil {
		return []string{}
	}
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}

func assertReason(t *testing.T, res mcpio.Message, want, label string) {
	t.Helper()
	if res.Error == nil {
		t.Fatalf("%s: expected deny %s, got allow", label, want)
	}
	got := rpcErrorReason(t, res.Error)
	if got != want {
		t.Fatalf("%s: reason_code=%s want %s data=%v", label, got, want, rpcErrorData(t, res.Error))
	}
}

func rpcErrorReason(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	data := rpcErrorData(t, raw)
	s, _ := data["reason_code"].(string)
	return s
}

func reasonDiff(t *testing.T, res mcpio.Message) map[string]any {
	t.Helper()
	data := rpcErrorData(t, res.Error)
	diff, _ := data["diff"].(map[string]any)
	return diff
}

func diffEmpty(diff map[string]any) bool {
	if diff == nil {
		return true
	}
	added, _ := diff["added"].([]any)
	removed, _ := diff["removed"].([]any)
	changed, _ := diff["changed"].([]any)
	return len(added) == 0 && len(removed) == 0 && len(changed) == 0
}
