package sidecar

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/furyheimdall/toolfence-deadbugz-guard/internal/mockmcp"
)

func TestWrapPinMissingIsToolCallFailure(t *testing.T) {
	cli, dir, stop := startWrap(t, wrapOpts{pin: false})
	defer stop()
	res := mustRPC(t, cli, 1, "tools/list", map[string]any{})
	if res.Error == nil {
		t.Fatal("PIN_MISSING must fail-closed")
	}
	var obj struct {
		Code    int            `json:"code"`
		Message string         `json:"message"`
		Data    map[string]any `json:"data"`
	}
	if err := json.Unmarshal(res.Error, &obj); err != nil {
		t.Fatal(err)
	}
	if obj.Data["reason_code"] != "PIN_MISSING" {
		t.Fatalf("data=%v", obj.Data)
	}
	if !strings.Contains(obj.Message, "PIN_MISSING") {
		t.Fatalf("message should name PIN_MISSING: %q", obj.Message)
	}
	if obj.Data["pin_path"] == "" || obj.Data["approve_hint"] == "" {
		t.Fatalf("visible pin_path/hint missing: %v", obj.Data)
	}
	if _, err := os.Stat(filepath.Join(dir, "pin.json.pending")); err != nil {
		t.Fatalf("pending snapshot: %v", err)
	}
}

func TestWrapApproveBootstrapsWithoutTTY(t *testing.T) {
	cli, dir, stop := startWrap(t, wrapOpts{pin: false, approve: true})
	defer stop()
	res := mustRPC(t, cli, 1, "tools/list", map[string]any{})
	if res.Error != nil {
		t.Fatalf("bootstrap should allow: %s", string(res.Error))
	}
	st, err := os.Stat(filepath.Join(dir, "pin.json"))
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != PinFileMode {
		t.Fatalf("mode=%o", st.Mode().Perm())
	}
}

func TestWrapReloadSeesOutOfBandApprove(t *testing.T) {
	cli, dir, stop := startWrap(t, wrapOpts{pin: false})
	defer stop()
	denied := mustRPC(t, cli, 1, "tools/list", map[string]any{})
	if denied.Error == nil {
		t.Fatal("expected PIN_MISSING")
	}
	pin := filepath.Join(dir, "pin.json")
	if _, err := ApproveFromPending(pin, ""); err != nil {
		t.Fatal(err)
	}
	ok := mustRPC(t, cli, 2, "tools/list", map[string]any{})
	if ok.Error != nil {
		t.Fatalf("after non-TTY approve, list should allow: %s", string(ok.Error))
	}
}

type wrapOpts struct {
	pin     bool
	approve bool
}

func startWrap(t *testing.T, opt wrapOpts) (*rpcClient, string, func()) {
	t.Helper()
	dir := t.TempDir()
	flip := filepath.Join(dir, "flip.json")
	atomicFlip(t, flip, mockmcp.ModeBenign)
	pinPath := filepath.Join(dir, "pin.json")
	if opt.pin {
		if _, err := WritePinFile(pinPath, "smoke-1", mockmcp.Tools(mockmcp.ModeBenign)); err != nil {
			t.Fatal(err)
		}
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
			ServerName:   "filesystem",
			Approve:      opt.approve,
		}, guardStdin, guardStdout, func() (io.WriteCloser, io.ReadCloser, func() error, error) {
			return wrapServerIn, wrapServerOut, func() error { return nil }, nil
		}, nil)
	}()

	cli := &rpcClient{in: cliToGuard, out: bufio.NewReader(cliFromGuard)}
	init := mustRPC(t, cli, 0, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "smoke", "version": "0"},
	})
	if init.Error != nil {
		t.Fatalf("initialize: %s", string(init.Error))
	}

	return cli, dir, func() {
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

