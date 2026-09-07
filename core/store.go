package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var (
	errPinMissing  = errors.New("pin missing")
	errPinTampered = errors.New("pin tampered")
)

// FileStore persists one pin JSON file. Load never writes the file.
type FileStore struct {
	Path string
}

// Load returns the pin or errPinMissing / errPinTampered. It does not create
// or repair a pin (Chief I: silent re-pin is forbidden).
func (f *FileStore) Load() (*Pin, error) {
	data, err := os.ReadFile(f.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, errPinMissing
	}
	if err != nil {
		return nil, err
	}
	var p Pin
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("%w: %v", errPinTampered, err)
	}
	if err := VerifyPin(p); err != nil {
		return nil, err
	}
	return clonePin(p), nil
}

// Save atomically replaces the pin file. Evaluate never calls Save.
func (f *FileStore) Save(p Pin) error {
	if err := VerifyPin(p); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(f.Path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(f.Path), ".pin-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, f.Path); err != nil {
		return err
	}
	if err := os.Chmod(f.Path, 0o600); err != nil {
		return err
	}
	ok = true
	return nil
}

// ParseToolsList accepts a tools/list snapshot: {"tools":[...]} or a bare array.
func ParseToolsList(r io.Reader) ([]ToolDef, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return ParseToolsListBytes(raw)
}

// ParseToolsListBytes parses a tools/list JSON snapshot.
func ParseToolsListBytes(raw []byte) ([]ToolDef, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty tools/list snapshot")
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	if trimmed[0] == '[' {
		var tools []ToolDef
		if err := dec.Decode(&tools); err != nil {
			return nil, fmt.Errorf("parse tools array: %w", err)
		}
		return tools, nil
	}
	var wrap struct {
		Tools []ToolDef `json:"tools"`
	}
	if err := dec.Decode(&wrap); err != nil {
		return nil, fmt.Errorf("parse tools/list: %w", err)
	}
	if wrap.Tools == nil {
		return nil, fmt.Errorf("tools/list snapshot missing tools array")
	}
	return wrap.Tools, nil
}
