package sidecar

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/furyheimdall/toolfence-deadbugz-guard/core"
	"github.com/furyheimdall/toolfence-deadbugz-guard/internal/mockmcp"
)

// CLI-level official GitHub MCP attach shape: argv after `--` includes
// --toolsets / --tools like github-mcp-server. The downstream binary is
// mock-mcp-deadbugz (no GitHub credentials).

func TestCLIGitHubToolsetsInventoryChange(t *testing.T) {
	dir := t.TempDir()
	guard := filepath.Join(dir, "deadbugz-guard")
	mock := filepath.Join(dir, "github-mcp-server")
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	build := func(out, pkg string) {
		t.Helper()
		cmd := exec.Command("go", "build", "-o", out, pkg)
		cmd.Dir = root
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v\n%s", pkg, err, b)
		}
	}
	build(guard, "./cmd/deadbugz-guard")
	build(mock, "./cmd/mock-mcp-deadbugz")

	pin := filepath.Join(dir, "pins", "github.json")
	flip := filepath.Join(dir, "flip.json")
	if err := os.MkdirAll(filepath.Dir(pin), 0o700); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	approve := exec.CommandContext(ctx, guard, "approve", "--name", "github", "--pin", pin,
		"--", mock, "--toolsets", "repos")
	approve.Env = append(os.Environ(),
		"FLIP_PATH="+flip,
		"GITHUB_TOOLSETS=repos",
		"GITHUB_PERSONAL_ACCESS_TOKEN=should-not-enter-fingerprint",
	)
	if b, err := approve.CombinedOutput(); err != nil {
		t.Fatalf("approve --toolsets repos: %v\n%s", err, b)
	}
	raw, err := os.ReadFile(pin)
	if err != nil {
		t.Fatal(err)
	}
	var pinned core.Pin
	if err := json.Unmarshal(raw, &pinned); err != nil {
		t.Fatal(err)
	}
	if pinned.ConfigFingerprint == "" || pinned.PinID == "" {
		t.Fatalf("approve must stamp #21 identity: %+v", pinned)
	}
	oldFp := pinned.ConfigFingerprint

	cmd := exec.CommandContext(ctx, guard, "--name", "github", "--pin", pin,
		"--call-gate", "3", "--", mock, "--toolsets", "repos,issues")
	cmd.Env = append(os.Environ(),
		"FLIP_PATH="+flip,
		"GITHUB_TOOLSETS=repos,issues",
		"GITHUB_PERSONAL_ACCESS_TOKEN=should-not-enter-fingerprint",
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	cli := &rpcClient{in: stdin, out: bufio.NewReader(stdout)}
	init := mustRPC(t, cli, 1, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "github-cli", "version": "0"},
	})
	if init.Error != nil {
		t.Fatalf("initialize: %s", string(init.Error))
	}
	list := mustRPC(t, cli, 2, "tools/list", map[string]any{})
	assertReason(t, list, "config_or_inventory_changed", "CLI --toolsets repos → repos,issues")
	if rpcErrorReason(t, list.Error) == "tools_list_drift" {
		t.Fatal("CLI inventory change must not be tools_list_drift")
	}
	data := rpcErrorData(t, list.Error)
	diff, _ := data["diff"].(map[string]any)
	if liveFp, _ := diff["live_fingerprint"].(string); liveFp == "" || liveFp == oldFp {
		t.Fatalf("live fingerprint should change: old=%s diff=%v", oldFp, diff)
	}
}

func TestCLIGitHubSameConfigSilentMutateIsDrift(t *testing.T) {
	dir := t.TempDir()
	guard := filepath.Join(dir, "deadbugz-guard")
	mock := filepath.Join(dir, "github-mcp-server")
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	build := func(out, pkg string) {
		t.Helper()
		cmd := exec.Command("go", "build", "-o", out, pkg)
		cmd.Dir = root
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v\n%s", pkg, err, b)
		}
	}
	build(guard, "./cmd/deadbugz-guard")
	build(mock, "./cmd/mock-mcp-deadbugz")

	pin := filepath.Join(dir, "github.json")
	flip := filepath.Join(dir, "flip.json")
	writeFlipFile(t, flip, "benign")

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	approve := exec.CommandContext(ctx, guard, "approve", "--name", "github", "--pin", pin,
		"--", mock, "--toolsets", "repos")
	approve.Env = append(os.Environ(), "FLIP_PATH="+flip, "GITHUB_TOOLSETS=repos")
	if b, err := approve.CombinedOutput(); err != nil {
		t.Fatalf("approve: %v\n%s", err, b)
	}

	writeFlipFile(t, flip, "poison")
	cmd := exec.CommandContext(ctx, guard, "--name", "github", "--pin", pin,
		"--call-gate", "3", "--", mock, "--toolsets", "repos")
	cmd.Env = append(os.Environ(), "FLIP_PATH="+flip, "GITHUB_TOOLSETS=repos")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	cli := &rpcClient{in: stdin, out: bufio.NewReader(stdout)}
	if init := mustRPC(t, cli, 1, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "github-cli", "version": "0"},
	}); init.Error != nil {
		t.Fatalf("initialize: %s", string(init.Error))
	}
	list := mustRPC(t, cli, 2, "tools/list", map[string]any{})
	assertReason(t, list, "tools_list_drift", "same --toolsets + poison listing")
}

func writeFlipFile(t *testing.T, path, mode string) {
	t.Helper()
	if err := mockmcp.WriteFlip(path, mockmcp.FlipState{Mode: mode, CallGate: 3}); err != nil {
		t.Fatal(err)
	}
}
