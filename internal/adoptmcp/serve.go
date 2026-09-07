package adoptmcp

import (
	"bufio"
	"encoding/json"
	"io"
	"os"

	"github.com/furyheimdall/toolfence-deadbugz-guard/sidecar/mcpio"
)

// Serve is a stdio MCP server that advertises one official-shaped inventory.
func Serve(in io.Reader, out io.Writer, argv []string) error {
	if len(argv) == 0 {
		argv = os.Args
	}
	profile := ProfileFromArgv(argv)

	r := bufio.NewReader(in)
	w := mcpio.NewWriter(out)
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
		switch msg.Method {
		case "initialize":
			_ = w.WriteJSON(mcpio.Message{
				JSONRPC: "2.0",
				ID:      msg.ID,
				Result: mustJSON(map[string]any{
					"protocolVersion": "2024-11-05",
					"capabilities":    map[string]any{"tools": map[string]any{}},
					"serverInfo":      map[string]any{"name": "mock-mcp-adopt", "version": "0.1.0", "profile": profile},
				}),
			})
		case "tools/list":
			// Re-read os.Environ each list so GITHUB_TOOLSETS / ADOPT_POISON
			// can change across wrap processes (CI starts a new process).
			live := ToolsForProfile(profile, argv, os.Environ())
			_ = w.WriteJSON(mcpio.Message{
				JSONRPC: "2.0",
				ID:      msg.ID,
				Result:  mustJSON(map[string]any{"tools": live}),
			})
		case "tools/call":
			_ = w.WriteJSON(mcpio.Message{
				JSONRPC: "2.0",
				ID:      msg.ID,
				Result: mustJSON(map[string]any{
					"content": []map[string]any{{"type": "text", "text": "ok:" + profile}},
					"isError": false,
				}),
			})
		default:
			_ = w.WriteJSON(mcpio.Message{
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
