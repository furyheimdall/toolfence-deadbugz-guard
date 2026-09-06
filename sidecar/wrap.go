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
type Wrap struct {
	Cfg  Config
	Gate core.Gate

	clientOut io.Writer
	clientMu  sync.Mutex

	mu      sync.Mutex
	pending map[string]string // id -> method
	syncCh  chan mcpio.Message
	syncN   atomic.Uint64
}

// Run is the stdio wrap loop: client <-> this process <-> downstream server.
func Run(ctx context.Context, cfg Config, clientIn io.Reader, clientOut io.Writer, start StartFunc, g core.Gate) error {
	cfg = ConfigFromEnv(cfg)
	if g == nil {
		g = GateFromPinFile(cfg.PinPath)
	}
	stdin, stdout, wait, err := start()
	if err != nil {
		return err
	}
	defer stdin.Close()

	w := &Wrap{
		Cfg:       cfg,
		Gate:      g,
		clientOut: clientOut,
		pending:   map[string]string{},
		syncCh:    make(chan mcpio.Message, 4),
	}

	errCh := make(chan error, 2)
	go func() {
		errCh <- w.pumpServer(ctx, stdout, clientOut)
	}()
	go func() {
		errCh <- w.pumpClient(ctx, clientIn, stdin)
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

func (w *Wrap) pumpClient(ctx context.Context, in io.Reader, serverIn io.Writer) error {
	r := bufio.NewReader(in)
	sw := mcpio.NewWriter(serverIn)
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
		if err := w.handleClient(msg, sw); err != nil {
			return err
		}
	}
}

func (w *Wrap) handleClient(msg mcpio.Message, serverIn *mcpio.Writer) error {
	switch msg.Method {
	case "tools/list":
		w.track(msg.IDString(), "tools/list")
		return serverIn.WriteJSON(msg)
	case "tools/call":
		if w.Cfg.CallGate >= DefaultCallGate {
			live, err := w.syncToolsList(serverIn)
			if err != nil {
				return w.writeClientDeny(msg.ID, core.Deny(core.ReasonInternalError, nil))
			}
			dec := w.Gate.Evaluate(context.Background(), live)
			kind := "deny"
			if dec.Allowed {
				kind = "allow"
			}
			log.Printf("deadbugz call_gate=%d decision=%s reason=%s", w.Cfg.CallGate, kind, dec.ReasonCode)
			if !dec.Allowed {
				return w.writeClientDeny(msg.ID, dec)
			}
		}
		return serverIn.WriteJSON(msg)
	default:
		return serverIn.WriteJSON(msg)
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

		if method == "tools/list" && len(msg.Result) > 0 {
			tools, perr := parseTools(msg.Result)
			if perr == nil {
				dec := w.Gate.Evaluate(context.Background(), tools)
				if !dec.Allowed {
					log.Printf("deadbugz tools/list deny reason=%s", dec.ReasonCode)
					if err := w.writeClientDenyTo(cw, msg.ID, dec); err != nil {
						return err
					}
					continue
				}
			}
		}
		w.clientMu.Lock()
		err = cw.WriteJSON(msg)
		w.clientMu.Unlock()
		if err != nil {
			return err
		}
	}
}

func (w *Wrap) syncToolsList(serverIn *mcpio.Writer) ([]core.ToolDef, error) {
	n := w.syncN.Add(1)
	id, _ := json.Marshal(fmt.Sprintf("deadbugz-sync-%d", n))
	req := mcpio.Message{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "tools/list",
		Params:  json.RawMessage(`{}`),
	}
	w.track(req.IDString(), "deadbugz-sync")
	if err := serverIn.WriteJSON(req); err != nil {
		return nil, err
	}
	msg := <-w.syncCh
	if len(msg.Error) > 0 {
		return nil, fmt.Errorf("sync tools/list: %s", string(msg.Error))
	}
	return parseTools(msg.Result)
}

func (w *Wrap) track(id, method string) {
	if id == "" {
		return
	}
	w.mu.Lock()
	w.pending[id] = method
	w.mu.Unlock()
}

func (w *Wrap) writeClientDeny(id json.RawMessage, dec core.GateDecision) error {
	return w.writeClientDenyTo(mcpio.NewWriter(w.clientOut), id, dec)
}

func (w *Wrap) writeClientDenyTo(cw *mcpio.Writer, id json.RawMessage, dec core.GateDecision) error {
	w.clientMu.Lock()
	defer w.clientMu.Unlock()
	data := map[string]any{
		"reason_code": dec.ReasonCode,
		"call_gate":   w.Cfg.CallGate,
		"hitl":        w.Cfg.HITLEndpoint,
		"audit_path":  w.Cfg.AuditPath,
	}
	if dec.Diff != nil {
		data["diff"] = dec.Diff
	}
	errObj := map[string]any{
		"code":    failClosedCode,
		"message": "deadbugz-guard: fail-closed",
		"data":    data,
	}
	b, err := json.Marshal(errObj)
	if err != nil {
		return err
	}
	return cw.WriteJSON(mcpio.Message{JSONRPC: "2.0", ID: id, Error: b})
}

func parseTools(result json.RawMessage) ([]core.ToolDef, error) {
	var wrap struct {
		Tools []core.ToolDef `json:"tools"`
	}
	if err := json.Unmarshal(result, &wrap); err != nil {
		return nil, err
	}
	return wrap.Tools, nil
}
