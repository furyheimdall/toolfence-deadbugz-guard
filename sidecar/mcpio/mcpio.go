// Package mcpio reads and writes MCP JSON-RPC messages.
// Supports LSP-style Content-Length frames and newline-delimited JSON.
package mcpio

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Message is a JSON-RPC 2.0 object used on the MCP stdio pipe.
type Message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
}

// IDString returns a stable map key for a JSON-RPC id.
func (m Message) IDString() string {
	return string(bytes.TrimSpace(m.ID))
}

// ReadMessage reads one framed or NDJSON message.
func ReadMessage(r *bufio.Reader) ([]byte, error) {
	for {
		prefix, err := r.Peek(1)
		if err != nil {
			return nil, err
		}
		if prefix[0] == 'C' || prefix[0] == 'c' {
			return readFramed(r)
		}
		if prefix[0] == '{' {
			line, err := r.ReadBytes('\n')
			if err != nil && len(line) == 0 {
				return nil, err
			}
			line = bytes.TrimSpace(line)
			if len(line) == 0 {
				if err != nil {
					return nil, err
				}
				continue
			}
			return line, nil
		}
		if _, err := r.ReadByte(); err != nil {
			return nil, err
		}
	}
}

func readFramed(r *bufio.Reader) ([]byte, error) {
	var contentLen int
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "content-length:") {
			n, err := strconv.Atoi(strings.TrimSpace(line[len("Content-Length:"):]))
			if err != nil {
				return nil, fmt.Errorf("content-length: %w", err)
			}
			contentLen = n
		}
	}
	if contentLen <= 0 {
		return nil, fmt.Errorf("missing content-length")
	}
	buf := make([]byte, contentLen)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// Writer writes NDJSON messages (enough for MCP clients and this pilot).
type Writer struct {
	w io.Writer
}

func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

func (wr *Writer) WriteJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = wr.w.Write(append(b, '\n'))
	return err
}
