// Command smokej is a headless session helper for scripts/smoke.sh J (#34).
// It is not a product entrypoint (deadbugz-guard stays the wrap).
//
// Already-pinned wrap, mid-session FLIP_PATH poison while tools/list is
// in-flight. The wrap must hash-before-forward (deny the poisoned list)
// and deny the next tools/call. Host list_changed is never sent.
//
//	smokej --guard PATH --mock PATH --pin PATH --flip PATH [--timeout 20s]
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/furyheimdall/toolfence-deadbugz-guard/internal/mockmcp"
	"github.com/furyheimdall/toolfence-deadbugz-guard/sidecar/mcpio"
)

func main() {
	timeout := flag.Duration("timeout", 20*time.Second, "overall deadline")
	guard := flag.String("guard", "", "deadbugz-guard binary")
	mock := flag.String("mock", "", "mock-mcp-deadbugz binary")
	pin := flag.String("pin", "", "already-written pin file")
	flip := flag.String("flip", "", "FLIP_PATH")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: smokej --guard PATH --mock PATH --pin PATH --flip PATH [--timeout 20s]\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *guard == "" || *mock == "" || *pin == "" || *flip == "" {
		flag.Usage()
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	out, err := runJ(ctx, *guard, *mock, *pin, *flip)
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smokej: %v\n", err)
		_ = enc.Encode(map[string]any{"ok": false, "error": err.Error()})
		os.Exit(1)
	}
	out["ok"] = true
	_ = enc.Encode(out)
}

func runJ(ctx context.Context, guard, mock, pin, flip string) (map[string]any, error) {
	if err := mockmcp.WriteFlip(flip, mockmcp.FlipState{Mode: mockmcp.ModeBenign, CallGate: 3}); err != nil {
		return nil, fmt.Errorf("flip benign: %w", err)
	}

	cmd := exec.CommandContext(ctx, guard, "--pin", pin, "--call-gate", "3", "--name", "smoke-j", "--", mock)
	cmd.Env = append(os.Environ(), "FLIP_PATH="+flip)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start wrap: %w", err)
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	w := mcpio.NewWriter(stdin)
	r := bufio.NewReader(stdout)

	if err := rpcOK(ctx, w, r, 1, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "smokej", "version": "0"},
	}); err != nil {
		return nil, fmt.Errorf("initialize: %w", err)
	}
	_ = w.WriteJSON(mcpio.Message{JSONRPC: "2.0", Method: "notifications/initialized", Params: json.RawMessage(`{}`)})

	if err := rpcOK(ctx, w, r, 2, "tools/call", map[string]any{
		"name":      "alpha",
		"arguments": map[string]any{},
	}); err != nil {
		return nil, fmt.Errorf("pre-poison tools/call should allow: %w", err)
	}

	if err := mockmcp.WriteFlip(flip, mockmcp.FlipState{Mode: mockmcp.ModeBenign, CallGate: 3, ListHold: true}); err != nil {
		return nil, fmt.Errorf("flip list_hold: %w", err)
	}

	listID, _ := json.Marshal(3)
	if err := w.WriteJSON(mcpio.Message{
		JSONRPC: "2.0",
		ID:      listID,
		Method:  "tools/list",
		Params:  json.RawMessage(`{}`),
	}); err != nil {
		return nil, fmt.Errorf("tools/list write: %w", err)
	}

	if err := waitHeld(ctx, flip); err != nil {
		return nil, err
	}

	if err := mockmcp.WriteFlip(flip, mockmcp.FlipState{Mode: mockmcp.ModePoison, CallGate: 3}); err != nil {
		return nil, fmt.Errorf("flip poison: %w", err)
	}

	list, err := readRPC(ctx, r, string(listID))
	if err != nil {
		return nil, fmt.Errorf("in-flight tools/list: %w", err)
	}
	if len(list.Error) == 0 {
		return nil, fmt.Errorf("in-flight poisoned tools/list must not be forwarded")
	}
	listReason := reasonFromError(list.Error)
	if listReason != "DIFF_NONEMPTY" && listReason != "APPROVAL_PENDING" {
		return nil, fmt.Errorf("in-flight list reason=%s want DIFF_NONEMPTY", listReason)
	}
	if looksLikePoisonedList(list.Result) {
		return nil, fmt.Errorf("hash-before-forward: poisoned tools/list leaked to host")
	}

	call, err := writeRPC(ctx, w, r, 4, "tools/call", map[string]any{
		"name":      "bravo",
		"arguments": map[string]any{},
	})
	if err != nil {
		return nil, fmt.Errorf("post-poison tools/call: %w", err)
	}
	if len(call.Error) == 0 {
		return nil, fmt.Errorf("subsequent tools/call must be denied")
	}
	callReason := reasonFromError(call.Error)
	if callReason != "DIFF_NONEMPTY" && callReason != "APPROVAL_PENDING" {
		return nil, fmt.Errorf("subsequent call reason=%s", callReason)
	}

	fmt.Fprintf(os.Stderr, "smokej: in-flight list deny reason=%s; subsequent call deny reason=%s\n", listReason, callReason)
	return map[string]any{
		"list_reason": listReason,
		"call_reason": callReason,
	}, nil
}

func waitHeld(ctx context.Context, flip string) error {
	held := mockmcp.HeldPath(flip)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		if _, err := os.Stat(held); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for in-flight list_hold marker")
		case <-tick.C:
		}
	}
}

func rpcOK(ctx context.Context, w *mcpio.Writer, r *bufio.Reader, id int, method string, params any) error {
	msg, err := writeRPC(ctx, w, r, id, method, params)
	if err != nil {
		return err
	}
	if len(msg.Error) > 0 {
		return fmt.Errorf("%s denied: %s", method, strings.TrimSpace(string(msg.Error)))
	}
	return nil
}

func writeRPC(ctx context.Context, w *mcpio.Writer, r *bufio.Reader, id int, method string, params any) (mcpio.Message, error) {
	pb, err := json.Marshal(params)
	if err != nil {
		return mcpio.Message{}, err
	}
	rawID, _ := json.Marshal(id)
	if err := w.WriteJSON(mcpio.Message{JSONRPC: "2.0", ID: rawID, Method: method, Params: pb}); err != nil {
		return mcpio.Message{}, err
	}
	return readRPC(ctx, r, string(rawID))
}

func readRPC(ctx context.Context, r *bufio.Reader, wantID string) (mcpio.Message, error) {
	type res struct {
		msg mcpio.Message
		err error
	}
	ch := make(chan res, 1)
	go func() {
		for {
			raw, err := mcpio.ReadMessage(r)
			if err != nil {
				ch <- res{err: err}
				return
			}
			var msg mcpio.Message
			if err := json.Unmarshal(raw, &msg); err != nil {
				continue
			}
			if msg.Method != "" && len(msg.Result) == 0 && len(msg.Error) == 0 {
				continue
			}
			if wantID != "" && msg.IDString() != wantID {
				continue
			}
			ch <- res{msg: msg}
			return
		}
	}()
	select {
	case <-ctx.Done():
		return mcpio.Message{}, ctx.Err()
	case out := <-ch:
		return out.msg, out.err
	}
}

func reasonFromError(raw json.RawMessage) string {
	var obj struct {
		Message string `json:"message"`
		Data    struct {
			Reason string `json:"reason_code"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		if obj.Data.Reason != "" {
			return obj.Data.Reason
		}
		if i := strings.LastIndex(obj.Message, " "); i >= 0 {
			return strings.TrimSpace(obj.Message[i+1:])
		}
	}
	return "ERROR"
}

func looksLikePoisonedList(result json.RawMessage) bool {
	if len(result) == 0 {
		return false
	}
	var wrap struct {
		Tools []struct {
			Description string `json:"description"`
		} `json:"tools"`
	}
	if json.Unmarshal(result, &wrap) != nil {
		return false
	}
	for _, t := range wrap.Tools {
		if strings.Contains(strings.ToUpper(t.Description), "POISON") {
			return true
		}
	}
	return false
}
