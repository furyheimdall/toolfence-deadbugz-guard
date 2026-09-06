package mockmcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// DefaultCallGate is Chief E / issue #7: call_gate=3.
const DefaultCallGate = 3

// FlipState is the FLIP_PATH fixture (Chief spec on PR #11).
// Changes must be published with WriteFlip (temp file → os.Rename only).
type FlipState struct {
	Mode     string `json:"mode"`
	CallGate int    `json:"call_gate"`
}

// DefaultFlip is benign + call_gate=3.
func DefaultFlip() FlipState {
	return FlipState{Mode: ModeBenign, CallGate: DefaultCallGate}
}

// ReadFlip loads FLIP_PATH. Missing/invalid file falls back to MODE / CALL_GATE / benign.
func ReadFlip(path string) FlipState {
	st := DefaultFlip()
	if path != "" {
		if b, err := os.ReadFile(path); err == nil {
			if parsed, ok := parseFlip(b); ok {
				return parsed
			}
		}
	}
	if m := os.Getenv("MODE"); m != "" {
		st.Mode = NormalizeMode(m)
	}
	if v := os.Getenv("CALL_GATE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			st.CallGate = n
		}
	}
	return st
}

func parseFlip(b []byte) (FlipState, bool) {
	st := DefaultFlip()
	trim := strings.TrimSpace(string(b))
	if trim == "" {
		return st, false
	}
	if json.Unmarshal(b, &st) == nil && (st.Mode != "" || st.CallGate != 0) {
		st.Mode = NormalizeMode(st.Mode)
		if st.CallGate <= 0 {
			st.CallGate = DefaultCallGate
		}
		return st, true
	}
	// key=value lines: mode=poison\ncall_gate=3
	mode, gate, saw := "", 0, false
	for _, line := range strings.Split(trim, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			if NormalizeMode(line) == line && (line == ModeBenign || line == ModePoison || line == ModeAdd || line == ModeRemove || line == ModeReorder || line == ModeDeadbugz) {
				mode, saw = line, true
			}
			continue
		}
		switch strings.TrimSpace(k) {
		case "mode":
			mode, saw = strings.TrimSpace(v), true
		case "call_gate":
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
				gate, saw = n, true
			}
		}
	}
	if !saw {
		return st, false
	}
	st.Mode = NormalizeMode(mode)
	if gate > 0 {
		st.CallGate = gate
	}
	return st, true
}

// WriteFlip publishes a new fixture via temp write + os.Rename only.
// It never truncates the live path in place.
func WriteFlip(path string, st FlipState) error {
	if path == "" {
		return fmt.Errorf("empty FLIP_PATH")
	}
	st.Mode = NormalizeMode(st.Mode)
	if st.CallGate <= 0 {
		st.CallGate = DefaultCallGate
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".flip-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}
