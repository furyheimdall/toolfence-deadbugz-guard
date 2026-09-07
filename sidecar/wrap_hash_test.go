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

// Mid-session mutations must fail-closed on tools/call without a client
// tools/list refresh (#22). Host list_changed is not part of this path.

func TestMidSessionPoisonFailClosedWithoutClientRefresh(t *testing.T) {
	assertMidSessionCallDenied(t, mockmcp.ModePoison, "poison")
}

func TestMidSessionAddFailClosedWithoutClientRefresh(t *testing.T) {
	assertMidSessionCallDenied(t, mockmcp.ModeAdd, "add")
}

func TestMidSessionRemoveFailClosedWithoutClientRefresh(t *testing.T) {
	assertMidSessionCallDenied(t, mockmcp.ModeRemove, "remove")
}

func assertMidSessionCallDenied(t *testing.T, mode, label string) {
	t.Helper()
	cli, flip, stop := startStack(t, mockmcp.ModeBenign)
	defer stop()

	ok := mustRPC(t, cli, 1, "tools/call", map[string]any{"name": "alpha", "arguments": map[string]any{}})
	if ok.Error != nil {
		t.Fatalf("pre-mutation call should allow: %s", string(ok.Error))
	}

	atomicFlip(t, flip, mode)
	// No client tools/list — call-gate sync hashes the live inventory.
	denied := mustRPC(t, cli, 2, "tools/call", map[string]any{"name": "alpha", "arguments": map[string]any{}})
	if denied.Error == nil {
		t.Fatalf("%s mid-session tools/call should fail-closed without client refresh", label)
	}
	data := rpcErrorData(t, denied.Error)
	if data["reason_code"] != "DIFF_NONEMPTY" && data["reason_code"] != "APPROVAL_PENDING" {
		t.Fatalf("%s reason_code=%v data=%v", label, data["reason_code"], data)
	}
}

func TestE_WrapDeadbugzCallGatePoisonBlocked(t *testing.T) {
	// Prior scenario E: Deadbugz + call_gate=3 poison is blocked on tools/call.
	cli, flip, stop := startStack(t, mockmcp.ModeBenign)
	defer stop()

	ok := mustRPC(t, cli, 1, "tools/call", map[string]any{"name": "alpha", "arguments": map[string]any{}})
	if ok.Error != nil {
		t.Fatalf("E pre-poison: %s", string(ok.Error))
	}
	atomicFlip(t, flip, mockmcp.ModeDeadbugz)
	denied := mustRPC(t, cli, 2, "tools/call", map[string]any{"name": "alpha", "arguments": map[string]any{}})
	if denied.Error == nil {
		t.Fatal("E: deadbugz call_gate poison must block tools/call")
	}
	data := rpcErrorData(t, denied.Error)
	if data["reason_code"] == "" {
		t.Fatalf("E missing reason_code: %v", data)
	}
	if int(data["call_gate"].(float64)) != 3 {
		t.Fatalf("E call_gate=%v", data["call_gate"])
	}
}

func TestJ_WrapInFlightPoisonDeniesNewCalls(t *testing.T) {
	// Prior scenario J: in-flight poison denies subsequent calls.
	cli, flip, stop := startStack(t, mockmcp.ModeBenign)
	defer stop()

	if d := mustRPC(t, cli, 1, "tools/call", map[string]any{"name": "alpha", "arguments": map[string]any{}}); d.Error != nil {
		t.Fatalf("J pre-poison: %s", string(d.Error))
	}
	atomicFlip(t, flip, mockmcp.ModePoison)
	if d := mustRPC(t, cli, 2, "tools/call", map[string]any{"name": "bravo", "arguments": map[string]any{}}); d.Error == nil {
		t.Fatal("J: in-flight poison must deny new calls")
	}
}

func TestServerListChangedRefreshHashedBeforeForward(t *testing.T) {
	cli, flip, notify, stop := startStackNotify(t, mockmcp.ModeBenign)
	defer stop()

	list := mustRPC(t, cli, 1, "tools/list", map[string]any{})
	if list.Error != nil {
		t.Fatalf("benign list: %s", string(list.Error))
	}

	atomicFlip(t, flip, mockmcp.ModePoison)
	notify <- struct{}{}
	n := waitListChanged(t, cli)
	if !isListChangedMethod(n.Method) {
		t.Fatalf("expected list_changed after hash, got %s", n.Method)
	}

	// Host re-list after list_changed must be hashed before forward (deny, not poisoned tools).
	denied := mustRPC(t, cli, 2, "tools/list", map[string]any{})
	if denied.Error == nil {
		t.Fatal("post-list_changed tools/list must not forward a poisoned listing")
	}
	if rpcErrorData(t, denied.Error)["reason_code"] == "" {
		t.Fatal("post-list_changed deny missing reason_code")
	}
}

func TestServerListChangedFailClosedWithoutClientRefresh(t *testing.T) {
	cli, flip, notify, stop := startStackNotify(t, mockmcp.ModeBenign)
	defer stop()

	if d := mustRPC(t, cli, 1, "tools/call", map[string]any{"name": "alpha", "arguments": map[string]any{}}); d.Error != nil {
		t.Fatalf("pre-list_changed call: %s", string(d.Error))
	}

	atomicFlip(t, flip, mockmcp.ModeAdd)
	notify <- struct{}{}
	_ = waitListChanged(t, cli)

	// No client tools/list after the notification.
	denied := mustRPC(t, cli, 2, "tools/call", map[string]any{"name": "alpha", "arguments": map[string]any{}})
	if denied.Error == nil {
		t.Fatal("list_changed must not skip the gate; call should fail-closed without client refresh")
	}
}

func TestHostListChangedIsNotSecurityBoundary(t *testing.T) {
	cli, flip, stop := startStack(t, mockmcp.ModeBenign)
	defer stop()

	if d := mustRPC(t, cli, 1, "tools/list", map[string]any{}); d.Error != nil {
		t.Fatalf("benign list: %s", string(d.Error))
	}

	atomicFlip(t, flip, mockmcp.ModePoison)
	sendNotification(t, cli, "notifications/tools/list_changed")

	denied := mustRPC(t, cli, 2, "tools/call", map[string]any{"name": "alpha", "arguments": map[string]any{}})
	if denied.Error == nil {
		t.Fatal("host list_changed must not open the gate for a poisoned inventory")
	}
	list := mustRPC(t, cli, 3, "tools/list", map[string]any{})
	if list.Error == nil {
		t.Fatal("host list_changed must not cause a poisoned tools/list to be forwarded")
	}
}

func TestUnparseableToolsListFailClosed(t *testing.T) {
	dir := t.TempDir()
	pinPath := filepath.Join(dir, "pin.json")
	if _, err := WritePinFile(pinPath, "smoke-1", mockmcp.Tools(mockmcp.ModeBenign)); err != nil {
		t.Fatal(err)
	}

	srvIn, wrapOut := io.Pipe()
	wrapIn, srvOut := io.Pipe()
	guardStdin, cliToGuard := io.Pipe()
	cliFromGuard, guardStdout := io.Pipe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		r := bufio.NewReader(srvIn)
		w := mcpio.NewWriter(srvOut)
		for {
			raw, err := mcpio.ReadMessage(r)
			if err != nil {
				return
			}
			var msg mcpio.Message
			if json.Unmarshal(raw, &msg) != nil {
				continue
			}
			switch msg.Method {
			case "initialize":
				_ = w.WriteJSON(mcpio.Message{
					JSONRPC: "2.0",
					ID:      msg.ID,
					Result:  json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{},"serverInfo":{"name":"bad-list","version":"0"}}`),
				})
			case "tools/list":
				_ = w.WriteJSON(mcpio.Message{
					JSONRPC: "2.0",
					ID:      msg.ID,
					Result:  json.RawMessage(`{"tools":"not-an-array"}`),
				})
			default:
				_ = w.WriteJSON(mcpio.Message{
					JSONRPC: "2.0",
					ID:      msg.ID,
					Result:  json.RawMessage(`{}`),
				})
			}
		}
	}()
	go func() {
		_ = Run(ctx, Config{
			PinPath:      pinPath,
			AuditPath:    filepath.Join(dir, "audit.jsonl"),
			HITLEndpoint: "http://127.0.0.1:9/hitl",
			CallGate:     3,
		}, guardStdin, guardStdout, func() (io.WriteCloser, io.ReadCloser, func() error, error) {
			return wrapOut, wrapIn, func() error { return nil }, nil
		}, nil)
	}()

	cli := &rpcClient{in: cliToGuard, out: bufio.NewReader(cliFromGuard)}
	if init := mustRPC(t, cli, 0, "initialize", map[string]any{"protocolVersion": "2024-11-05"}); init.Error != nil {
		t.Fatalf("initialize: %s", string(init.Error))
	}
	denied := mustRPC(t, cli, 1, "tools/list", map[string]any{})
	if denied.Error == nil {
		t.Fatal("unparseable tools/list must fail-closed, not forward")
	}
	if rpcErrorData(t, denied.Error)["reason_code"] != "INTERNAL_ERROR" {
		t.Fatalf("unparseable reason: %v", rpcErrorData(t, denied.Error))
	}
}

func TestIsListChangedMethod(t *testing.T) {
	if !isListChangedMethod("notifications/tools/list_changed") || !isListChangedMethod("tools/list_changed") {
		t.Fatal("expected both MCP list_changed method names")
	}
	if isListChangedMethod("tools/list") || isListChangedMethod("notifications/initialized") {
		t.Fatal("tools/list is not list_changed")
	}
}

func waitListChanged(t *testing.T, cli *rpcClient) mcpio.Message {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		type result struct {
			raw []byte
			err error
		}
		ch := make(chan result, 1)
		go func() {
			raw, err := mcpio.ReadMessage(cli.out)
			ch <- result{raw, err}
		}()
		select {
		case r := <-ch:
			if r.err != nil {
				t.Fatal(r.err)
			}
			var msg mcpio.Message
			if err := json.Unmarshal(r.raw, &msg); err != nil {
				t.Fatal(err)
			}
			if isListChangedMethod(msg.Method) {
				return msg
			}
		case <-deadline:
			t.Fatal("timeout waiting for list_changed")
		}
	}
}

func sendNotification(t *testing.T, cli *rpcClient, method string) {
	t.Helper()
	req := mcpio.Message{JSONRPC: "2.0", Method: method}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cli.in.Write(append(b, '\n')); err != nil {
		t.Fatal(err)
	}
}
