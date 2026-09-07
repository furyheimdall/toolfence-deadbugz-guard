// Command mcpprobe is a headless MCP stdio client for CI verify scripts.
// It is not a product entrypoint (deadbugz-guard stays the wrap).
//
//	mcpprobe [--expect-allow|--expect-deny [REASON]] [--timeout 60s] -- <cmd> [args...]
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

	"github.com/furyheimdall/toolfence-deadbugz-guard/sidecar/mcpio"
)

func main() {
	timeout := flag.Duration("timeout", 60*time.Second, "overall deadline")
	expectAllow := flag.Bool("expect-allow", false, "tools/list must succeed (pin match)")
	expectDeny := flag.String("expect-deny", "", "tools/list must fail-closed; optional reason_code (PIN_MISSING, DIFF_NONEMPTY, ...)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: mcpprobe [--expect-allow|--expect-deny [REASON]] [--timeout 60s] -- <cmd> [args...]\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	argv := flag.Args()
	if len(argv) == 0 {
		flag.Usage()
		os.Exit(2)
	}
	if *expectAllow && *expectDeny != "" {
		fmt.Fprintln(os.Stderr, "mcpprobe: use only one of --expect-allow / --expect-deny")
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	out, err := probe(ctx, argv)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mcpprobe: %v\n", err)
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"ok": false, "error": err.Error()})
		os.Exit(1)
	}

	ok := true
	switch {
	case *expectAllow:
		if !out.Allowed {
			ok = false
			fmt.Fprintf(os.Stderr, "mcpprobe: expected allow, got deny reason=%s\n", out.Reason)
		}
	case *expectDeny != "":
		if out.Allowed {
			ok = false
			fmt.Fprintln(os.Stderr, "mcpprobe: expected deny, got allow")
		} else if *expectDeny != "any" && !strings.EqualFold(out.Reason, *expectDeny) {
			ok = false
			fmt.Fprintf(os.Stderr, "mcpprobe: expected deny reason=%s, got %s\n", *expectDeny, out.Reason)
		}
	}
	out.OK = ok
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(out)
	if !ok {
		os.Exit(1)
	}
}

type result struct {
	OK      bool     `json:"ok"`
	Allowed bool     `json:"allowed"`
	Reason  string   `json:"reason_code,omitempty"`
	Tools   []string `json:"tools,omitempty"`
	Error   string   `json:"error,omitempty"`
}

func probe(ctx context.Context, argv []string) (result, error) {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return result{}, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return result{}, err
	}
	if err := cmd.Start(); err != nil {
		return result{}, err
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	w := mcpio.NewWriter(stdin)
	r := bufio.NewReader(stdout)

	initID, _ := json.Marshal(1)
	initParams, _ := json.Marshal(map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "deadbugz-verify", "version": "e3"},
	})
	if err := w.WriteJSON(mcpio.Message{JSONRPC: "2.0", ID: initID, Method: "initialize", Params: initParams}); err != nil {
		return result{}, fmt.Errorf("initialize write: %w", err)
	}
	if _, err := readRPC(ctx, r, string(initID)); err != nil {
		return result{}, fmt.Errorf("initialize: %w", err)
	}
	_ = w.WriteJSON(mcpio.Message{JSONRPC: "2.0", Method: "notifications/initialized", Params: json.RawMessage(`{}`)})

	listID, _ := json.Marshal(2)
	if err := w.WriteJSON(mcpio.Message{JSONRPC: "2.0", ID: listID, Method: "tools/list", Params: json.RawMessage(`{}`)}); err != nil {
		return result{}, fmt.Errorf("tools/list write: %w", err)
	}
	msg, err := readRPC(ctx, r, string(listID))
	if err != nil {
		return result{}, fmt.Errorf("tools/list: %w", err)
	}
	if len(msg.Error) > 0 {
		reason := reasonFromError(msg.Error)
		fmt.Fprintf(os.Stderr, "mcpprobe: tools/list deny reason=%s\n", reason)
		return result{Allowed: false, Reason: reason, Error: strings.TrimSpace(string(msg.Error))}, nil
	}
	names, err := toolNames(msg.Result)
	if err != nil {
		return result{}, err
	}
	if len(names) == 0 {
		return result{}, fmt.Errorf("tools/list returned no tools")
	}
	fmt.Fprintf(os.Stderr, "mcpprobe: tools/list allow n=%d\n", len(names))
	return result{Allowed: true, Tools: names}, nil
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

func toolNames(result json.RawMessage) ([]string, error) {
	var wrap struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(result, &wrap); err != nil {
		return nil, fmt.Errorf("parse tools: %w", err)
	}
	names := make([]string, 0, len(wrap.Tools))
	for _, t := range wrap.Tools {
		if t.Name != "" {
			names = append(names, t.Name)
		}
	}
	return names, nil
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
