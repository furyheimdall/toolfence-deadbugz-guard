package sidecar

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/furyheimdall/toolfence-deadbugz-guard/core"
)

// Thin adapters over the merged #12 core contract.
// Do not redefine ToolDiffSummary / GateDecision / Gate / Pin here.

// WritePinFile hashes tools with core.PinTools and writes a core.Pin JSON file.
func WritePinFile(path, version string, tools []core.ToolDef) (core.Pin, error) {
	if version == "" {
		version = "smoke-1"
	}
	p := core.PinTools(tools, version)
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return core.Pin{}, err
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		return core.Pin{}, fmt.Errorf("write pin: %w", err)
	}
	return p, nil
}

// GateFromPinFile installs a core.MemoryGate from a pin file.
// Missing / unreadable pin yields an empty fail-closed gate (PIN_MISSING).
func GateFromPinFile(path string) core.Gate {
	g := core.NewMemoryGate()
	b, err := os.ReadFile(path)
	if err != nil {
		return g
	}
	var p core.Pin
	if err := json.Unmarshal(b, &p); err != nil {
		return g
	}
	if p.Aggregate == "" {
		return g
	}
	_ = g.InstallInitialPin(p)
	return g
}
