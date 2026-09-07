package sidecar

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"

	"github.com/furyheimdall/toolfence-deadbugz-guard/core"
	"github.com/furyheimdall/toolfence-deadbugz-guard/sidecar/mcpio"
)

func bindProcessConfig(g core.Gate, cfg Config) {
	if g == nil || len(cfg.ServerArgv) == 0 {
		return
	}
	aware, ok := g.(core.ConfigAware)
	if !ok {
		return
	}
	aware.SetConfig(core.NewConfigIdentity(cfg.ServerArgv, os.Environ()))
}

const failClosedCode = -32003

// StartFunc starts the downstream MCP server and returns its stdio pipes.
type StartFunc func() (stdin io.WriteCloser, stdout io.ReadCloser, wait func() error, err error)

// ExecStart starts argv as the wrapped server (deadbugz-guard -- <server>).
func ExecStart(argv []string) StartFunc {
	return func() (io.WriteCloser, io.ReadCloser, func() error, error) {
		if len(argv) == 0 {
			return nil, nil, nil, fmt.Errorf("missing server after --")
		}
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Stderr = os.Stderr
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return nil, nil, nil, err
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return nil, nil, nil, err
		}
		if err := cmd.Start(); err != nil {
			return nil, nil, nil, err
		}
		return stdin, stdout, cmd.Wait, nil
	}
}

// Wrap observes tools/list (and tools/call when CallGate>=3) on a stdio MCP pipe.
//
// Every tools/list result is hashed/diffed before it is forwarded. Server
// notifications/tools/list_changed trigger a guard-owned refresh that is also
// hashed before anything reaches the host. Host list_changed / deferred-tool
// refresh is not a security boundary (#22).
type Wrap struct {
	Cfg  Config
	Gate core.Gate

	clientOut io.Writer
	clientMu  sync.Mutex

	serverIn *mcpio.Writer
	serverMu sync.Mutex

	mu      sync.Mutex
	pending map[string]string // id -> method
	syncCh  chan mcpio.Message
	syncMu  sync.Mutex // one in-flight deadbugz-sync at a time
	syncN   atomic.Uint64

	gateMu sync.Mutex
}

// Run is the stdio wrap loop: client <-> this process <-> downstream server.
func Run(ctx context.Context, cfg Config, clientIn io.Reader, clientOut io.Writer, start StartFunc, g core.Gate) error {
	cfg = ConfigFromEnv(cfg)
	if g == nil {
		g = GateFromPinFile(cfg.PinPath)
	}
	bindProcessConfig(g, cfg)
	stdin, stdout, wait, err := start()
	if err != nil {
		return err
	}
	defer stdin.Close()

	w := &Wrap{
		Cfg:       cfg,
		Gate:      g,
		clientOut: clientOut,
		serverIn:  mcpio.NewWriter(stdin),
		pending:   map[string]string{},
		syncCh:    make(chan mcpio.Message, 4),
	}

	errCh := make(chan error, 2)
	go func() {
		errCh <- w.pumpServer(ctx, stdout, clientOut)
	}()
	go func() {
		errCh <- w.pumpClient(ctx, clientIn)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		if wait != nil {
			_ = wait()
		}
		if err == io.EOF {
			return nil
		}
		return err
	}
}

func (w *Wrap) pumpClient(ctx context.Context, in io.Reader) error {
	r := bufio.NewReader(in)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		raw, err := mcpio.ReadMessage(r)
		if err != nil {
			return err
		}
		var msg mcpio.Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		if err := w.handleClient(msg); err != nil {
			return err
		}
	}
}

func (w *Wrap) handleClient(msg mcpio.Message) error {
	switch {
	case isListChangedMethod(msg.Method):
		// Host list_changed / deferred-tool refresh is not a security
		// boundary. Pass the notification through; the gate still hashes
		// the next live inventory (list or call-gate sync) itself.
		log.Printf("deadbugz host %s ignored as security boundary", msg.Method)
		return w.writeServer(msg)
	case msg.Method == "tools/list":
		w.track(msg.IDString(), "tools/list")
		return w.writeServer(msg)
	case msg.Method == "tools/call":
		if w.Cfg.CallGate >= DefaultCallGate {
			live, err := w.syncToolsList()
			if err != nil {
				return w.writeClientDeny(msg.ID, core.Deny(core.ReasonInternalError, nil))
			}
			dec := w.evaluateLive(live)
			kind := "deny"
			if dec.Allowed {
				kind = "allow"
			}
			log.Printf("deadbugz call_gate=%d decision=%s reason=%s", w.Cfg.CallGate, kind, dec.ReasonCode)
			if !dec.Allowed {
				return w.writeClientDeny(msg.ID, dec)
			}
		}
		return w.writeServer(msg)
	default:
		return w.writeServer(msg)
	}
}

func (w *Wrap) pumpServer(ctx context.Context, serverOut io.Reader, clientOut io.Writer) error {
	r := bufio.NewReader(serverOut)
	cw := mcpio.NewWriter(clientOut)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		raw, err := mcpio.ReadMessage(r)
		if err != nil {
			return err
		}
		var msg mcpio.Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			if err := cw.WriteJSON(json.RawMessage(raw)); err != nil {
				return err
			}
			continue
		}

		if isListChangedMethod(msg.Method) && len(msg.ID) == 0 {
			// Server list_changed is not a security decision. Refresh
			// and hash before the host sees a listing (or treats this
			// notification as authoritative).
			go w.handleServerListChanged(ctx, cw, msg)
			continue
		}

		id := msg.IDString()
		w.mu.Lock()
		method := w.pending[id]
		if method == "deadbugz-sync" {
			delete(w.pending, id)
			w.mu.Unlock()
			select {
			case w.syncCh <- msg:
			case <-ctx.Done():
				return ctx.Err()
			}
			continue
		}
		delete(w.pending, id)
		w.mu.Unlock()

		if deny, dec := w.denyToolsListForward(msg, method); deny {
			w.logDeny(dec)
			if err := w.writeClientDenyTo(cw, msg.ID, dec); err != nil {
				return err
			}
			continue
		}
		w.clientMu.Lock()
		err = cw.WriteJSON(msg)
		w.clientMu.Unlock()
		if err != nil {
			return err
		}
	}
}

// handleServerListChanged hashes a guard-owned tools/list refresh, then
// forwards the notification. The notification itself never opens the gate.
func (w *Wrap) handleServerListChanged(ctx context.Context, cw *mcpio.Writer, msg mcpio.Message) {
	live, err := w.syncToolsList()
	if err != nil {
		log.Printf("deadbugz list_changed refresh failed: %v", err)
	} else {
		dec := w.evaluateLive(live)
		kind := "deny"
		if dec.Allowed {
			kind = "allow"
		}
		log.Printf("deadbugz list_changed refresh decision=%s reason=%s", kind, dec.ReasonCode)
	}
	if err := ctx.Err(); err != nil {
		return
	}
	w.clientMu.Lock()
	defer w.clientMu.Unlock()
	_ = cw.WriteJSON(msg)
}

func (w *Wrap) syncToolsList() ([]core.ToolDef, error) {
	w.syncMu.Lock()
	defer w.syncMu.Unlock()

	n := w.syncN.Add(1)
	id, _ := json.Marshal(fmt.Sprintf("deadbugz-sync-%d", n))
	req := mcpio.Message{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "tools/list",
		Params:  json.RawMessage(`{}`),
	}
	w.track(req.IDString(), "deadbugz-sync")
	if err := w.writeServer(req); err != nil {
		return nil, err
	}
	msg := <-w.syncCh
	if len(msg.Error) > 0 {
		return nil, fmt.Errorf("sync tools/list: %s", string(msg.Error))
	}
	tools, err := parseTools(msg.Result)
	if err != nil {
		return nil, err
	}
	return tools, nil
}

// denyToolsListForward hashes/diffs a tools/list result before the host sees
// it. Parse failure is fail-closed (INTERNAL_ERROR) — never forward an
// unhashed listing.
func (w *Wrap) denyToolsListForward(msg mcpio.Message, pendingMethod string) (bool, core.GateDecision) {
	if pendingMethod != "tools/list" && !looksLikeToolsListResult(msg) {
		return false, core.GateDecision{}
	}
	if len(msg.Error) > 0 {
		return false, core.GateDecision{}
	}
	if len(msg.Result) == 0 {
		return true, core.Deny(core.ReasonInternalError, nil)
	}
	tools, err := parseTools(msg.Result)
	if err != nil {
		return true, core.Deny(core.ReasonInternalError, nil)
	}
	dec := w.evaluateLive(tools)
	if !dec.Allowed {
		return true, dec
	}
	return false, dec
}

func (w *Wrap) writeServer(v any) error {
	w.serverMu.Lock()
	defer w.serverMu.Unlock()
	if w.serverIn == nil {
		return fmt.Errorf("server stdin not attached")
	}
	return w.serverIn.WriteJSON(v)
}

func (w *Wrap) track(id, method string) {
	if id == "" {
		return
	}
	w.mu.Lock()
	w.pending[id] = method
	w.mu.Unlock()
}

func (w *Wrap) evaluateLive(live []core.ToolDef) core.GateDecision {
	w.gateMu.Lock()
	defer w.gateMu.Unlock()
	// Re-read the pin file so `deadbugz-guard approve` (no TTY) is visible
	// on the next tools/list without an interactive prompt.
	if w.Cfg.PinPath != "" {
		w.Gate = GateFromPinFile(w.Cfg.PinPath)
		bindProcessConfig(w.Gate, w.Cfg)
	}
	if w.Gate == nil {
		return core.Deny(core.ReasonInternalError, nil)
	}
	dec := w.Gate.Evaluate(context.Background(), live)
	if dec.Allowed {
		return dec
	}
	if p, ok, err := ApplyNonTTYApprove(w.Cfg, dec.ReasonCode, live); err != nil {
		log.Printf("deadbugz-guard non-TTY approve failed: %v", err)
	} else if ok {
		log.Printf("deadbugz-guard non-TTY approve wrote pin %s hash=%s mode=0600", w.Cfg.PinPath, p.Aggregate)
		w.Gate = GateFromPinFile(w.Cfg.PinPath)
		bindProcessConfig(w.Gate, w.Cfg)
		dec = w.Gate.Evaluate(context.Background(), live)
		if dec.Allowed {
			return dec
		}
	}
	if err := WritePendingSnapshot(w.Cfg.PinPath, dec.ReasonCode, live, dec.Diff); err != nil {
		log.Printf("deadbugz-guard pending snapshot: %v", err)
	}
	return dec
}

func (w *Wrap) logDeny(dec core.GateDecision) {
	log.Printf("deadbugz-guard deny reason=%s pin=%s name=%s (Cursor: Output panel → MCP Logs). non-TTY approve: deadbugz-guard approve --pin %s -- -- <server>",
		dec.ReasonCode, w.Cfg.PinPath, w.Cfg.ServerName, w.Cfg.PinPath)
}

func (w *Wrap) writeClientDeny(id json.RawMessage, dec core.GateDecision) error {
	w.logDeny(dec)
	return w.writeClientDenyTo(mcpio.NewWriter(w.clientOut), id, dec)
}

func (w *Wrap) writeClientDenyTo(cw *mcpio.Writer, id json.RawMessage, dec core.GateDecision) error {
	w.clientMu.Lock()
	defer w.clientMu.Unlock()
	hint := fmt.Sprintf("non-TTY: deadbugz-guard approve --pin %s -- -- <server>; or DEADBUGZ_APPROVE=1 for first PIN_MISSING only. Cursor: Output → MCP Logs", w.Cfg.PinPath)
	data := map[string]any{
		"reason_code":  dec.ReasonCode,
		"call_gate":    w.Cfg.CallGate,
		"hitl":         w.Cfg.HITLEndpoint,
		"audit_path":   w.Cfg.AuditPath,
		"pin_path":     w.Cfg.PinPath,
		"server_name":  w.Cfg.ServerName,
		"approve_hint": hint,
	}
	if dec.Diff != nil {
		data["diff"] = dec.Diff
	}
	errObj := map[string]any{
		"code":    failClosedCode,
		"message": fmt.Sprintf("deadbugz-guard: %s", dec.ReasonCode),
		"data":    data,
	}
	b, err := json.Marshal(errObj)
	if err != nil {
		return err
	}
	return cw.WriteJSON(mcpio.Message{JSONRPC: "2.0", ID: id, Error: b})
}

func parseTools(result json.RawMessage) ([]core.ToolDef, error) {
	if len(result) == 0 {
		return nil, fmt.Errorf("empty tools/list result")
	}
	var wrap struct {
		Tools []core.ToolDef `json:"tools"`
	}
	if err := json.Unmarshal(result, &wrap); err != nil {
		return nil, err
	}
	return wrap.Tools, nil
}

func looksLikeToolsListResult(msg mcpio.Message) bool {
	if len(msg.Result) == 0 {
		return false
	}
	var wrap struct {
		Tools json.RawMessage `json:"tools"`
	}
	if err := json.Unmarshal(msg.Result, &wrap); err != nil {
		return false
	}
	return len(wrap.Tools) > 0 && wrap.Tools[0] == '['
}

func isListChangedMethod(method string) bool {
	switch method {
	case "notifications/tools/list_changed", "tools/list_changed":
		return true
	default:
		return false
	}
}
