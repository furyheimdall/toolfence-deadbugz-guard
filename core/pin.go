package core

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// PinTools is a thin test stub for E1's production pin builder.
// Hash = SHA-256 over canonical JSON of tools sorted by name, each
// object using name + description + inputSchema. Reorder-only input
// yields the same Aggregate.
func PinTools(tools []ToolDef, version string) Pin {
	sorted := append([]ToolDef(nil), tools...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})

	hashes := make(map[string]string, len(sorted))
	canon := make([]map[string]any, 0, len(sorted))
	for _, t := range sorted {
		schema := json.RawMessage(t.InputSchema)
		if len(schema) == 0 {
			schema = json.RawMessage("null")
		}
		entry := map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"inputSchema": json.RawMessage(schema),
		}
		canon = append(canon, entry)
		hashes[t.Name] = hashJSON(entry)
	}

	return Pin{
		Version:    version,
		Aggregate:  hashJSON(canon),
		ToolHashes: hashes,
		CreatedAt:  time.Now().UTC(),
	}
}

func hashJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		sum := sha256.Sum256([]byte(fmt.Sprintf("%v", v)))
		return hex.EncodeToString(sum[:])
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
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
	return &out
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
