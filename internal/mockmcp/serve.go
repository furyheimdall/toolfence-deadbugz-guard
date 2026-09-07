package mockmcp

import (
	"bufio"
	"encoding/json"
	"io"
	"sync"

	"github.com/furyheimdall/toolfence-deadbugz-guard/sidecar/mcpio"
)

// Serve reads MCP JSON-RPC from in and writes responses to out.
// Mode is FLIP_PATH (atomic file) or MODE, default benign.
func Serve(in io.Reader, out io.Writer, flipPath string) error {
	return ServeWithNotify(in, out, flipPath, nil)
}

// ServeWithNotify is Serve plus an optional channel that emits
// notifications/tools/list_changed (test hook for #22).
func ServeWithNotify(in io.Reader, out io.Writer, flipPath string, notify <-chan struct{}) error {
	r := bufio.NewReader(in)
	w := mcpio.NewWriter(out)
	var mu sync.Mutex
	write := func(v any) error {
		mu.Lock()
		defer mu.Unlock()
		return w.WriteJSON(v)
	}

	if notify != nil {
		go func() {
			for range notify {
				_ = write(mcpio.Message{
					JSONRPC: "2.0",
					Method:  "notifications/tools/list_changed",
				})
			}
		}()
	}

	for {
		raw, err := mcpio.ReadMessage(r)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		var msg mcpio.Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		if msg.Method == "" || len(msg.ID) == 0 {
			continue
		}
		st := ReadFlip(flipPath)
		switch msg.Method {
		case "initialize":
			_ = write(mcpio.Message{
				JSONRPC: "2.0",
				ID:      msg.ID,
				Result: mustJSON(map[string]any{
					"protocolVersion": "2024-11-05",
					"capabilities":    map[string]any{"tools": map[string]any{}},
					"serverInfo":      map[string]any{"name": "mock-mcp-deadbugz", "version": "0.1.0"},
				}),
			})
		case "tools/list":
			_ = write(mcpio.Message{
				JSONRPC: "2.0",
				ID:      msg.ID,
				Result:  mustJSON(map[string]any{"tools": Tools(st.Mode)}),
			})
		case "tools/call":
			_ = write(mcpio.Message{
				JSONRPC: "2.0",
				ID:      msg.ID,
				Result: mustJSON(map[string]any{
					"content":   []map[string]any{{"type": "text", "text": "ok:" + st.Mode}},
					"isError":   false,
					"call_gate": st.CallGate,
					"mode":      st.Mode,
				}),
			})
		default:
			_ = write(mcpio.Message{
				JSONRPC: "2.0",
				ID:      msg.ID,
				Error:   mustJSON(map[string]any{"code": -32601, "message": "method not found"}),
			})
		}
	}
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
