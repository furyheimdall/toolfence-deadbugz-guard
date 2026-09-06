package sidecar

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSmokeCLIStdioWrap(t *testing.T) {
	dir := t.TempDir()
	guard := filepath.Join(dir, "deadbugz-guard")
	mock := filepath.Join(dir, "mock-mcp-deadbugz")
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

	pin := filepath.Join(dir, "pin.json")
	write := exec.Command(guard, "--write-pin", "--pin", pin, "--from-mode", "benign")
	if b, err := write.CombinedOutput(); err != nil {
		t.Fatalf("write-pin: %v\n%s", err, b)
	}

	cmd := exec.Command(guard, "--pin", pin, "--audit", filepath.Join(dir, "audit.jsonl"),
		"--hitl", "http://127.0.0.1:9/hitl", "--call-gate", "3", "--", mock)
	cmd.Env = append(os.Environ(), "MODE=benign")
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
		"clientInfo":      map[string]any{"name": "cli-smoke", "version": "0"},
	})
	if init.Error != nil {
		t.Fatalf("initialize: %s", string(init.Error))
	}
	list := mustRPC(t, cli, 2, "tools/list", map[string]any{})
	if list.Error != nil {
		t.Fatalf("CLI wrap tools/list should allow: %s", string(list.Error))
	}
	var body struct {
		Tools []struct{ Name string } `json:"tools"`
	}
	if err := json.Unmarshal(list.Result, &body); err != nil || len(body.Tools) != 2 {
		t.Fatalf("tools: %+v err=%v", body, err)
	}
}
