package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// CanonicalJSON returns compact JSON with object keys sorted recursively
// (encoding/json map encoding) and array order preserved.
func CanonicalJSON(v any) ([]byte, error) {
	norm, err := normalize(v)
	if err != nil {
		return nil, err
	}
	return json.Marshal(norm)
}

// CanonicalToolJSON is name + description + inputSchema (+ annotations).
func CanonicalToolJSON(t ToolDef) ([]byte, error) {
	payload, err := toolPayload(t)
	if err != nil {
		return nil, err
	}
	return CanonicalJSON(payload)
}

func toolPayload(t ToolDef) (map[string]any, error) {
	schema, err := decodeJSON(t.InputSchema)
	if err != nil {
		return nil, fmt.Errorf("inputSchema: %w", err)
	}
	if schema == nil {
		schema = map[string]any{}
	}
	m := map[string]any{
		"name":        t.Name,
		"description": t.Description,
		"inputSchema": schema,
	}
	if len(bytes.TrimSpace(t.Annotations)) > 0 && !isJSONNull(t.Annotations) {
		ann, err := decodeJSON(t.Annotations)
		if err != nil {
			return nil, fmt.Errorf("annotations: %w", err)
		}
		if ann != nil {
			m["annotations"] = ann
		}
	}
	return m, nil
}

func hashCanonical(v any) (string, error) {
	raw, err := CanonicalJSON(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func decodeJSON(raw json.RawMessage) (any, error) {
	if len(bytes.TrimSpace(raw)) == 0 || isJSONNull(raw) {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return normalize(v)
}

func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func normalize(v any) (any, error) {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			nv, err := normalize(val)
			if err != nil {
				return nil, err
			}
			out[k] = nv
		}
		return out, nil
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			nv, err := normalize(val)
			if err != nil {
				return nil, err
			}
			out[i] = nv
		}
		return out, nil
	case json.RawMessage:
		return decodeJSON(x)
	case json.Number:
		return x, nil
	default:
		return x, nil
	}
}
