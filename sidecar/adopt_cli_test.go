package sidecar

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/furyheimdall/toolfence-deadbugz-guard/core"
	"github.com/furyheimdall/toolfence-deadbugz-guard/internal/adoptmcp"
	"github.com/furyheimdall/toolfence-deadbugz-guard/sidecar/mcpio"
)

// #23: scripted/CI Filesystem+Fetch attach and GitHub toolsets inventory
// change. Uses mock-mcp-adopt (official-shaped stub). Does not expand
// smoke G/H/I/J.

var adoptBins struct {
	once  sync.Once
	dir   string
	err   error
	guard string
	stub  string
}

func adoptBinaries(t *testing.T) (guard, stub string) {
	t.Helper()
	adoptBins.once.Do(func() {
		root, err := filepath.Abs("..")
		if err != nil {
			adoptBins.err = err
			return
		}
		dir, err := os.MkdirTemp("", "deadbugz-adopt-")
		if err != nil {
			adoptBins.err = err
			return
		}
		adoptBins.dir = dir
		adoptBins.guard = filepath.Join(dir, "deadbugz-guard")
		adoptBins.stub = filepath.Join(dir, "mock-mcp-adopt")
		build := func(out, pkg string) {
			if adoptBins.err != nil {
				return
			}
			cmd := exec.Command("go", "build", "-o", out, pkg)
			cmd.Dir = root
			if b, err := cmd.CombinedOutput(); err != nil {
				adoptBins.err = err
				adoptBins.dir = string(b)
			}
		}
		build(adoptBins.guard, "./cmd/deadbugz-guard")
		build(adoptBins.stub, "./cmd/mock-mcp-adopt")
	})
	if adoptBins.err != nil {
		t.Fatalf("build adopt bins: %v\n%s", adoptBins.err, adoptBins.dir)
	}
	return adoptBins.guard, adoptBins.stub
}

func TestAttachFilesystemFetchNonTTYApprove(t *testing.T) {
	guard, stub := adoptBinaries(t)
	dir := t.TempDir()
	allowed := filepath.Join(dir, "allowed")
	if err := os.MkdirAll(allowed, 0o700); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		argv []string
		tool string
	}{
		{"filesystem", []string{stub, "filesystem", allowed}, "read_text_file"},
		{"fetch", []string{stub, "fetch"}, "fetch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pin := filepath.Join(dir, tc.name+".json")
			approve := exec.Command(guard, "approve", "--name", tc.name, "--pin", pin, "--")
			approve.Args = append(approve.Args, tc.argv...)
			if b, err := approve.CombinedOutput(); err != nil {
				t.Fatalf("approve: %v\n%s", err, b)
			}
			st, err := os.Stat(pin)
			if err != nil {
				t.Fatal(err)
			}
			if st.Mode().Perm() != PinFileMode {
				t.Fatalf("pin mode=%o", st.Mode().Perm())
			}
			p := mustReadPin(t, pin)
			if p.ConfigFingerprint == "" || p.PinID == "" || p.ServerName != tc.name {
				t.Fatalf("approve must stamp #21 identity: %+v", p)
			}

			res := listThroughWrap(t, guard, pin, tc.name, nil, tc.argv)
			if res.Error != nil {
				t.Fatalf("pinned wrap should allow: %s", string(res.Error))
			}
			if !rpcHasTool(t, res, tc.tool) {
				t.Fatalf("expected tool %s in listing", tc.tool)
			}
		})
	}
}

func TestAttachIDESpawnApproveFlagBootstraps(t *testing.T) {
	guard, stub := adoptBinaries(t)
	dir := t.TempDir()
	pin := filepath.Join(dir, "filesystem.json")
	argv := []string{stub, "filesystem", dir}

	missing := listThroughWrap(t, guard, pin, "filesystem", nil, argv)
	if missing.Error == nil {
		t.Fatal("PIN_MISSING must fail-closed without a grant")
	}
	data := rpcErrorData(t, missing.Error)
	if data["reason_code"] != "PIN_MISSING" {
		t.Fatalf("want PIN_MISSING, got %v", data["reason_code"])
	}

	boot := listThroughWrap(t, guard, pin, "filesystem", []string{"--approve"}, argv)
	if boot.Error != nil {
		t.Fatalf("DEADBUGZ_APPROVE-style flag should bootstrap: %s", string(boot.Error))
	}
	st, err := os.Stat(pin)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != PinFileMode {
		t.Fatalf("bootstrap pin mode=%o", st.Mode().Perm())
	}
	p := mustReadPin(t, pin)
	if p.ConfigFingerprint == "" {
		t.Fatal("bootstrap pin must carry config_fingerprint")
	}
}

func TestGitHubToolsetsConfigOrInventoryChanged(t *testing.T) {
	guard, stub := adoptBinaries(t)
	dir := t.TempDir()
	pin := filepath.Join(dir, "github.json")
	repos := []string{stub, "github", "--toolsets", "repos"}

	approve := exec.Command(guard, "approve", "--name", "github", "--pin", pin, "--")
	approve.Args = append(approve.Args, repos...)
	if b, err := approve.CombinedOutput(); err != nil {
		t.Fatalf("approve repos: %v\n%s", err, b)
	}
	pinned := mustReadPin(t, pin)
	if pinned.ConfigFingerprint == "" {
		t.Fatal("github pin must have config_fingerprint")
	}

	ok := listThroughWrap(t, guard, pin, "github", nil, repos)
	if ok.Error != nil {
		t.Fatalf("same toolsets should allow: %s", string(ok.Error))
	}
	if !rpcHasTool(t, ok, "get_file_contents") || rpcHasTool(t, ok, "list_issues") {
		t.Fatal("repos pin should list repos tools only")
	}

	changed := listThroughWrap(t, guard, pin, "github", nil, []string{stub, "github", "--toolsets", "repos,issues"})
	if changed.Error == nil {
		t.Fatal("intentional --toolsets change must fail-closed")
	}
	data := rpcErrorData(t, changed.Error)
	if data["reason_code"] != string(core.ReasonConfigOrInventoryChanged) {
		t.Fatalf("want config_or_inventory_changed, got %v (must not be tools_list_drift)", data["reason_code"])
	}

	envChange := listThroughWrapEnv(t, guard, pin, "github", nil, repos, []string{"GITHUB_TOOLSETS=repos,issues"})
	if envChange.Error == nil {
		t.Fatal("GITHUB_TOOLSETS change must fail-closed")
	}
	envData := rpcErrorData(t, envChange.Error)
	if envData["reason_code"] != string(core.ReasonConfigOrInventoryChanged) {
		t.Fatalf("env inventory: want config_or_inventory_changed, got %v", envData["reason_code"])
	}
}

func TestGitHubSameToolsetsPoisonIsToolsListDrift(t *testing.T) {
	guard, stub := adoptBinaries(t)
	dir := t.TempDir()
	pin := filepath.Join(dir, "github.json")
	repos := []string{stub, "github", "--toolsets", "repos"}

	approve := exec.Command(guard, "approve", "--name", "github", "--pin", pin, "--")
	approve.Args = append(approve.Args, repos...)
	if b, err := approve.CombinedOutput(); err != nil {
		t.Fatalf("approve: %v\n%s", err, b)
	}

	poison := listThroughWrapEnv(t, guard, pin, "github", nil, repos, []string{"ADOPT_POISON=1"})
	if poison.Error == nil {
		t.Fatal("same fingerprint + mutated list must fail-closed")
	}
	data := rpcErrorData(t, poison.Error)
	if data["reason_code"] != string(core.ReasonToolsListDrift) {
		t.Fatalf("want tools_list_drift, got %v", data["reason_code"])
	}

	// --approve must not auto-promote silent drift or inventory change.
	still := listThroughWrap(t, guard, pin, "github", []string{"--approve"}, []string{stub, "github", "--toolsets", "repos,issues"})
	if still.Error == nil {
		t.Fatal("--approve must not re-pin inventory change")
	}
	stillData := rpcErrorData(t, still.Error)
	if stillData["reason_code"] != string(core.ReasonConfigOrInventoryChanged) {
		t.Fatalf("flag must stay fail-closed: %v", stillData["reason_code"])
	}
}

func TestProcessIdentityPrefersServerName(t *testing.T) {
	ident := ProcessIdentity(Config{
		ServerName: "filesystem",
		ServerArgv: []string{"/ABS/npx", "-y", "@modelcontextprotocol/server-filesystem", "/ABS/allowed-dir"},
	})
	if ident.ServerName != "filesystem" {
		t.Fatalf("server name: %s", ident.ServerName)
	}
	if ident.Fingerprint() == "" {
		t.Fatal("empty fingerprint")
	}
	other := ProcessIdentity(Config{
		ServerName: "filesystem",
		ServerArgv: []string{"/ABS/npx", "-y", "@modelcontextprotocol/server-filesystem", "/other"},
	})
	if ident.Fingerprint() == other.Fingerprint() {
		t.Fatal("allowed-dir argv change must change fingerprint")
	}
}

func TestAdoptCatalogMatchesStub(t *testing.T) {
	if !hasAdoptTool(adoptmcp.FilesystemTools(), "read_text_file") {
		t.Fatal("filesystem catalog")
	}
	if !hasAdoptTool(adoptmcp.FetchTools(), "fetch") {
		t.Fatal("fetch catalog")
	}
}

func listThroughWrap(t *testing.T, guard, pin, name string, extra, argv []string) mcpio.Message {
	t.Helper()
	return listThroughWrapEnv(t, guard, pin, name, extra, argv, nil)
}

func listThroughWrapEnv(t *testing.T, guard, pin, name string, extra, argv, extraEnv []string) mcpio.Message {
	t.Helper()
	args := []string{"--name", name, "--pin", pin, "--call-gate", "3"}
	args = append(args, extra...)
	args = append(args, "--")
	args = append(args, argv...)
	cmd := exec.Command(guard, args...)
	cmd.Env = append(os.Environ(), extraEnv...)
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
		"clientInfo":      map[string]any{"name": "adopt-ci", "version": "0"},
	})
	if init.Error != nil {
		t.Fatalf("initialize: %s", string(init.Error))
	}
	return mustRPC(t, cli, 2, "tools/list", map[string]any{})
}

func mustReadPin(t *testing.T, path string) core.Pin {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var p core.Pin
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func rpcHasTool(t *testing.T, msg mcpio.Message, name string) bool {
	t.Helper()
	var body struct {
		Tools []struct{ Name string } `json:"tools"`
	}
	if err := json.Unmarshal(msg.Result, &body); err != nil {
		t.Fatal(err)
	}
	for _, tool := range body.Tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func hasAdoptTool(tools []core.ToolDef, name string) bool {
	for _, tool := range tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}
