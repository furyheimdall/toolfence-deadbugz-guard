package sidecar

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/furyheimdall/toolfence-deadbugz-guard/core"
	"github.com/furyheimdall/toolfence-deadbugz-guard/sidecar/mcpio"
)

// PinFileState is the on-disk pin, used to refuse silent repair of a tamper.
type PinFileState int

const (
	PinFileAbsent PinFileState = iota
	PinFileOK
	PinFileTampered
)

// InspectPinFile reports whether the pin is missing, readable, or tampered.
// Evaluate never writes the file.
func InspectPinFile(path string) PinFileState {
	if path == "" {
		return PinFileAbsent
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return PinFileAbsent
	}
	var p core.Pin
	if err := json.Unmarshal(data, &p); err != nil {
		return PinFileTampered
	}
	if err := core.VerifyPin(p); err != nil {
		return PinFileTampered
	}
	return PinFileOK
}

// tokenApproves reports whether a one-shot file asks to approve.
func tokenApproves(path string) bool {
	if path == "" {
		return false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	s := strings.ToLower(strings.TrimSpace(string(b)))
	return s == "" || s == "approve" || s == "yes" || s == "1" || s == "true" || s == "on"
}

func consumeFile(path string) {
	if path == "" {
		return
	}
	_ = os.Remove(path)
}

// httpAllowsBootstrap GETs url (2s). 200 + decision approve (or body "approve").
func httpAllowsBootstrap(ctx context.Context, rawURL string) bool {
	if rawURL == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return false
	}
	var obj struct {
		Decision string `json:"decision"`
	}
	if json.Unmarshal(body, &obj) == nil && strings.EqualFold(obj.Decision, "approve") {
		return true
	}
	s := strings.ToLower(strings.TrimSpace(string(body)))
	return s == "approve" || s == "yes" || s == "1" || s == "true"
}

// allowNonTTYApprove is the headless grant. Tampered pins never auto-repair.
// --approve / DEADBUGZ_APPROVE=1 / HTTP: first pin only (file absent).
// Approve-file token: first pin or explicit drift re-pin (consumed after write).
func allowNonTTYApprove(cfg Config, reason core.ReasonCode, state PinFileState) (ok bool, consumeToken bool) {
	if state == PinFileTampered {
		return false, false
	}
	fileOK := tokenApproves(cfg.ApproveFile)
	switch reason {
	case core.ReasonPinMissing:
		if state != PinFileAbsent {
			return false, false
		}
		if fileOK {
			return true, true
		}
		if cfg.Approve {
			return true, false
		}
		if cfg.ApproveHTTP != "" && httpAllowsBootstrap(context.Background(), cfg.ApproveHTTP) {
			return true, false
		}
		return false, false
	default:
		// DIFF_NONEMPTY plus #21 tools_list_drift / config_or_inventory_changed:
		// one-shot file only. Flag/HTTP never auto-promote drift.
		if reason.RequiresApproval() && fileOK {
			return true, true
		}
		return false, false
	}
}

// ApplyNonTTYApprove writes a durable 0600 pin when a headless grant is present.
func ApplyNonTTYApprove(cfg Config, reason core.ReasonCode, live []core.ToolDef) (core.Pin, bool, error) {
	if cfg.PinPath == "" {
		return core.Pin{}, false, fmt.Errorf("pin path required for approve")
	}
	state := InspectPinFile(cfg.PinPath)
	ok, consume := allowNonTTYApprove(cfg, reason, state)
	if !ok {
		return core.Pin{}, false, nil
	}
	p, err := WritePinFileWithConfig(cfg.PinPath, "pin-1", live, ProcessIdentity(cfg))
	if err != nil {
		return core.Pin{}, false, err
	}
	if consume {
		consumeFile(cfg.ApproveFile)
	}
	_ = os.Remove(PendingPath(cfg.PinPath))
	return p, true, nil
}

// PendingSnapshot is the fail-closed live catalog for `deadbugz-guard approve`.
type PendingSnapshot struct {
	Reason string                `json:"reason_code"`
	Pin    core.Pin              `json:"pin"`
	Diff   *core.ToolDiffSummary `json:"diff,omitempty"`
}

// WritePendingSnapshot stores the live tools so approve can run without a TTY
// and without talking to the IDE-spawned wrap.
func WritePendingSnapshot(pinPath string, reason core.ReasonCode, live []core.ToolDef, diff *core.ToolDiffSummary) error {
	if pinPath == "" {
		return nil
	}
	p := core.PinTools(live, "pending-1")
	snap := PendingSnapshot{Reason: string(reason), Pin: p, Diff: diff}
	return writeJSONFile(PendingPath(pinPath), snap)
}

// WritePendingSnapshotIdent is WritePendingSnapshot plus #21 identity.
func WritePendingSnapshotIdent(pinPath string, reason core.ReasonCode, live []core.ToolDef, diff *core.ToolDiffSummary, ident core.ConfigIdentity) error {
	if pinPath == "" {
		return nil
	}
	var p core.Pin
	if ident.ServerName != "" || len(ident.Argv) > 0 || len(ident.Env) > 0 {
		p = core.PinToolsWithConfig(live, "pending-1", ident)
	} else {
		p = core.PinTools(live, "pending-1")
	}
	snap := PendingSnapshot{Reason: string(reason), Pin: p, Diff: diff}
	return writeJSONFile(PendingPath(pinPath), snap)
}

// ApproveFromPending writes the durable pin from a pending snapshot.
func ApproveFromPending(pinPath, pendingPath string) (core.Pin, error) {
	if pendingPath == "" {
		pendingPath = PendingPath(pinPath)
	}
	data, err := os.ReadFile(pendingPath)
	if err != nil {
		return core.Pin{}, fmt.Errorf("pending snapshot: %w", err)
	}
	var snap PendingSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return core.Pin{}, fmt.Errorf("pending snapshot: %w", err)
	}
	if len(snap.Pin.Tools) == 0 {
		return core.Pin{}, fmt.Errorf("pending snapshot has no tools")
	}
	p := snap.Pin
	p.Version = "pin-1"
	if err := writeJSONFile(pinPath, p); err != nil {
		return core.Pin{}, err
	}
	_ = os.Remove(pendingPath)
	return p, nil
}

// ListToolsFromServer runs initialize + tools/list on a downstream stdio server.
func ListToolsFromServer(ctx context.Context, start StartFunc) ([]core.ToolDef, error) {
	stdin, stdout, wait, err := start()
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = stdin.Close()
		if wait != nil {
			done := make(chan struct{})
			go func() {
				_ = wait()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
			}
		}
		_ = stdout.Close()
	}()

	w := mcpio.NewWriter(stdin)
	r := bufio.NewReader(stdout)

	initID, _ := json.Marshal(1)
	initParams, _ := json.Marshal(map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "deadbugz-guard", "version": "approve"},
	})
	if err := w.WriteJSON(mcpio.Message{JSONRPC: "2.0", ID: initID, Method: "initialize", Params: initParams}); err != nil {
		return nil, err
	}
	if _, err := readRPCResult(ctx, r, string(initID)); err != nil {
		return nil, fmt.Errorf("initialize: %w", err)
	}
	_ = w.WriteJSON(mcpio.Message{JSONRPC: "2.0", Method: "notifications/initialized", Params: json.RawMessage(`{}`)})

	listID, _ := json.Marshal(2)
	if err := w.WriteJSON(mcpio.Message{JSONRPC: "2.0", ID: listID, Method: "tools/list", Params: json.RawMessage(`{}`)}); err != nil {
		return nil, err
	}
	msg, err := readRPCResult(ctx, r, string(listID))
	if err != nil {
		return nil, fmt.Errorf("tools/list: %w", err)
	}
	if len(msg.Error) > 0 {
		return nil, fmt.Errorf("tools/list: %s", string(msg.Error))
	}
	return parseTools(msg.Result)
}

// ApproveFromServer lists live tools and writes a 0600 pin (CLI / non-TTY).
func ApproveFromServer(ctx context.Context, pinPath string, start StartFunc) (core.Pin, error) {
	return ApproveFromServerConfig(ctx, Config{PinPath: pinPath}, start)
}

// ApproveFromServerConfig is ApproveFromServer plus #21 pin identity
// (argv after `--` and inventory env). Needed so a later toolsets /
// argv change is config_or_inventory_changed, not tools_list_drift.
func ApproveFromServerConfig(ctx context.Context, cfg Config, start StartFunc) (core.Pin, error) {
	if cfg.PinPath == "" {
		return core.Pin{}, fmt.Errorf("pin path required")
	}
	tools, err := ListToolsFromServer(ctx, start)
	if err != nil {
		return core.Pin{}, err
	}
	if len(tools) == 0 {
		return core.Pin{}, fmt.Errorf("tools/list returned no tools")
	}
	p, err := WritePinFileWithConfig(cfg.PinPath, "pin-1", tools, ProcessIdentity(cfg))
	if err != nil {
		return core.Pin{}, err
	}
	_ = os.Remove(PendingPath(cfg.PinPath))
	return p, nil
}

func readRPCResult(ctx context.Context, r *bufio.Reader, wantID string) (mcpio.Message, error) {
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
				continue // notification
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

func writeJSONFile(path string, v any) error {
	if path == "" {
		return fmt.Errorf("empty path")
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), PinDirMode); err != nil {
		return err
	}
	if err := os.WriteFile(path, append(b, '\n'), PinFileMode); err != nil {
		return err
	}
	return os.Chmod(path, PinFileMode)
}
