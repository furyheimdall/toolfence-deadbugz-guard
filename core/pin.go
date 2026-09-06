package core

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// PinTools builds a pin from a tools/list snapshot.
//
// Algorithm (Chief / #3):
//  1. Sort tools by name
//  2. Canonical JSON of name + description + inputSchema (+ annotations)
//  3. SHA-256 per tool and of the sorted catalog
//
// Reorder-only input (Chief D) yields the same Aggregate. This is the
// production pin builder; Evaluate never writes a pin.
func PinTools(tools []ToolDef, version string) Pin {
	sorted, err := sortToolsByName(tools)
	if err != nil {
		// Preserve a deterministic empty pin rather than panic; Evaluate
		// treats a bad catalog as INTERNAL_ERROR via HashCatalog.
		return Pin{Version: version, CreatedAt: time.Now().UTC()}
	}
	p, err := hashCatalog(sorted, version)
	if err != nil {
		return Pin{Version: version, CreatedAt: time.Now().UTC()}
	}
	return p
}

// HashCatalog is PinTools with an error. Used by the gate and file store.
func HashCatalog(tools []ToolDef, version string) (Pin, error) {
	sorted, err := sortToolsByName(tools)
	if err != nil {
		return Pin{}, err
	}
	return hashCatalog(sorted, version)
}

func hashCatalog(sorted []ToolDef, version string) (Pin, error) {
	hashes := make(map[string]string, len(sorted))
	payloads := make([]any, 0, len(sorted))
	for _, t := range sorted {
		payload, err := toolPayload(t)
		if err != nil {
			return Pin{}, fmt.Errorf("tool %q: %w", t.Name, err)
		}
		payloads = append(payloads, payload)
		h, err := hashCanonical(payload)
		if err != nil {
			return Pin{}, fmt.Errorf("tool %q: %w", t.Name, err)
		}
		hashes[t.Name] = h
	}
	agg, err := hashCanonical(payloads)
	if err != nil {
		return Pin{}, err
	}
	return Pin{
		Version:    version,
		Aggregate:  agg,
		ToolHashes: hashes,
		Tools:      sorted,
		CreatedAt:  time.Now().UTC(),
	}, nil
}

func sortToolsByName(tools []ToolDef) ([]ToolDef, error) {
	out := append([]ToolDef(nil), tools...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	seen := make(map[string]struct{}, len(out))
	for _, t := range out {
		if strings.TrimSpace(t.Name) == "" {
			return nil, fmt.Errorf("tool name is required")
		}
		if _, ok := seen[t.Name]; ok {
			return nil, fmt.Errorf("duplicate tool name %q", t.Name)
		}
		seen[t.Name] = struct{}{}
	}
	return out, nil
}

func nextPinVersion(current string, seq int) string {
	if current == "" {
		return fmt.Sprintf("pin-%d", seq)
	}
	return fmt.Sprintf("pin-%d", seq)
}

func clonePin(p Pin) *Pin {
	out := p
	if p.ToolHashes != nil {
		out.ToolHashes = make(map[string]string, len(p.ToolHashes))
		for k, v := range p.ToolHashes {
			out.ToolHashes[k] = v
		}
	}
	if p.Tools != nil {
		out.Tools = append([]ToolDef(nil), p.Tools...)
	}
	return &out
}

// Diff compares two hashed catalogs (pinned vs live). Reorder-only input
// that shares the same Aggregate produces an empty summary (Chief D).
func Diff(pinned, live Pin) ToolDiffSummary {
	return summarize(pinned, live)
}

func summarize(oldPin, livePin Pin) ToolDiffSummary {
	added, removed, changed := []string{}, []string{}, []string{}
	oldKeys := map[string]struct{}{}
	for name, h := range oldPin.ToolHashes {
		oldKeys[name] = struct{}{}
		liveH, ok := livePin.ToolHashes[name]
		if !ok {
			removed = append(removed, name)
			continue
		}
		if liveH != h {
			changed = append(changed, name)
		}
	}
	for name := range livePin.ToolHashes {
		if _, ok := oldKeys[name]; !ok {
			added = append(added, name)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	sort.Strings(changed)
	return ToolDiffSummary{
		Added:       added,
		Removed:     removed,
		Changed:     changed,
		PinRevision: oldPin.Version,
		LiveHash:    livePin.Aggregate,
		PinHash:     oldPin.Aggregate,
	}
}

// VerifyPin recomputes the catalog hash. Tamper or algorithm drift fails closed.
func VerifyPin(p Pin) error {
	if p.Aggregate == "" || p.Version == "" {
		return fmt.Errorf("%w: missing hash fields", errPinTampered)
	}
	if len(p.Tools) == 0 && len(p.ToolHashes) == 0 {
		got, err := HashCatalog(nil, p.Version)
		if err != nil {
			return fmt.Errorf("%w: %v", errPinTampered, err)
		}
		if got.Aggregate != p.Aggregate {
			return fmt.Errorf("%w: aggregate mismatch", errPinTampered)
		}
		return nil
	}
	if len(p.Tools) > 0 {
		got, err := HashCatalog(p.Tools, p.Version)
		if err != nil {
			return fmt.Errorf("%w: %v", errPinTampered, err)
		}
		if got.Aggregate != p.Aggregate {
			return fmt.Errorf("%w: aggregate mismatch", errPinTampered)
		}
		return nil
	}
	if !validSHA256Hex(p.Aggregate) {
		return fmt.Errorf("%w: invalid aggregate", errPinTampered)
	}
	for name, h := range p.ToolHashes {
		if name == "" || !validSHA256Hex(h) {
			return fmt.Errorf("%w: invalid tool hash", errPinTampered)
		}
	}
	return nil
}

func validSHA256Hex(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
