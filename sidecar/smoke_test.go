package sidecar

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/furyheimdall/toolfence-deadbugz-guard/internal/mockmcp"
	"github.com/furyheimdall/toolfence-deadbugz-guard/sidecar/mcpio"
)

func TestSmokeUnchangedAllow(t *testing.T) {
	cli, flip, stop := startStack(t, mockmcp.ModeBenign)
	defer stop()
	_ = flip
	res := mustRPC(t, cli, 1, "tools/list", map[string]any{})
	if res.Error != nil {
		t.Fatalf("unchanged tools/list should allow: %s", string(res.Error))
	}
}

func TestSmokeReorderAllow(t *testing.T) {
	cli, flip, stop := startStack(t, mockmcp.ModeBenign)
	defer stop()
	atomicWrite(t, flip, mockmcp.ModeReorder)
	res := mustRPC(t, cli, 1, "tools/list", map[string]any{})
	if res.Error != nil {
		t.Fatalf("reorder → allow, got deny: %s", string(res.Error))
	}
	var body struct {
		Tools []struct{ Name string } `json:"tools"`
	}
	if err := json.Unmarshal(res.Result, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %+v", body.Tools)
	}
}

func TestSmokePoisonDeny(t *testing.T) {
	cli, flip, stop := startStack(t, mockmcp.ModeBenign)
	defer stop()
	atomicWrite(t, flip, mockmcp.ModePoison)
	res := mustRPC(t, cli, 1, "tools/list", map[string]any{})
	if res.Error == nil {
		t.Fatal("poison → deny, got allow")
	}
	data := rpcErrorData(t, res.Error)
	if data["reason_code"] != "DIFF_NONEMPTY" {
		t.Fatalf("reason_code=%v data=%v", data["reason_code"], data)
	}
}

func TestSmokeCallGate3(t *testing.T) {
	cli, flip, stop := startStack(t, mockmcp.ModeBenign)
	defer stop()

	ok := mustRPC(t, cli, 1, "tools/call", map[string]any{"name": "alpha", "arguments": map[string]any{}})
	if ok.Error != nil {
		t.Fatalf("benign tools/call should allow: %s", string(ok.Error))
	}

	atomicWrite(t, flip, mockmcp.ModePoison)
	denied := mustRPC(t, cli, 2, "tools/call", map[string]any{"name": "alpha", "arguments": map[string]any{}})
	if denied.Error == nil {
		t.Fatal("call_gate=3 poison should deny tools/call")
	}
	data := rpcErrorData(t, denied.Error)
	gate, _ := data["call_gate"].(float64)
	if int(gate) != 3 {
		t.Fatalf("expected call_gate=3, data=%v", data)
	}
	if data["reason_code"] == "" {
		t.Fatalf("missing reason_code: %v", data)
	}
}

type rpcClient struct {
	in  io.Writer
	out *bufio.Reader
}

func startStack(t *testing.T, initial string) (*rpcClient, string, func()) {
	t.Helper()
	dir := t.TempDir()
	flip := filepath.Join(dir, "mode")
	atomicWrite(t, flip, initial)
	pinPath := filepath.Join(dir, "pin.json")
	if _, err := WritePinFile(pinPath, "smoke-1", mockmcp.Tools(mockmcp.ModeBenign)); err != nil {
		t.Fatal(err)
	}

	mockIn, wrapServerIn := io.Pipe()      // mock reads mockIn; wrap writes wrapServerIn
	wrapServerOut, mockOut := io.Pipe()    // wrap reads wrapServerOut; mock writes mockOut
	guardStdin, cliToGuard := io.Pipe()    // wrap reads guardStdin; client writes cliToGuard
	cliFromGuard, guardStdout := io.Pipe() // client reads cliFromGuard; wrap writes guardStdout

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

	return cli, flip, func() {
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

func mustRPC(t *testing.T, cli *rpcClient, id int, method string, params any) mcpio.Message {
	t.Helper()
	pb, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	rawID, _ := json.Marshal(id)
	req := mcpio.Message{JSONRPC: "2.0", ID: rawID, Method: method, Params: pb}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cli.in.Write(append(b, '\n')); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(5 * time.Second)
	type result struct {
		raw []byte
		err error
	}
	ch := make(chan result, 1)
	go func() {
		raw, err := mcpio.ReadMessage(cli.out)
		ch <- result{raw, err}
	}()
	var raw []byte
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatal(r.err)
		}
		raw = r.raw
	case <-deadline:
		t.Fatal("timeout waiting for " + method)
	}
	var msg mcpio.Message
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatal(err)
	}
	return msg
}

func rpcErrorData(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var obj struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	return obj.Data
}

func atomicWrite(t *testing.T, path, body string) {
	t.Helper()
	tmp := path + ".tmp"
	if err := writeFile(tmp, body+"\n"); err != nil {
		t.Fatal(err)
	}
	if err := renameFile(tmp, path); err != nil {
		t.Fatal(err)
	}
}
