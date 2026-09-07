package sidecar

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/furyheimdall/toolfence-deadbugz-guard/core"
)

// Thin adapters over the merged #12 core contract.
// Do not redefine ToolDiffSummary / GateDecision / Gate / Pin here.

// WritePinFile hashes tools with core.PinTools and writes a 0600 pin JSON file.
func WritePinFile(path, version string, tools []core.ToolDef) (core.Pin, error) {
	return WritePinFileWithConfig(path, version, tools, core.ConfigIdentity{})
}

// WritePinFileWithConfig is WritePinFile plus #21 pin identity when cfg
// has argv or an explicit server name.
func WritePinFileWithConfig(path, version string, tools []core.ToolDef, cfg core.ConfigIdentity) (core.Pin, error) {
	if version == "" {
		version = "smoke-1"
	}
	var p core.Pin
	if cfg.ServerName != "" || len(cfg.Argv) > 0 || len(cfg.Env) > 0 {
		p = core.PinToolsWithConfig(tools, version, cfg)
	} else {
		p = core.PinTools(tools, version)
	}
	if err := writeJSONFile(path, p); err != nil {
		return core.Pin{}, fmt.Errorf("write pin: %w", err)
	}
	return p, nil
}

// EnsurePinDir creates the parent of path at 0700 (idempotent).
func EnsurePinDir(path string) error {
	if path == "" {
		return fmt.Errorf("empty pin path")
	}
	return os.MkdirAll(filepath.Dir(path), PinDirMode)
}

// GateFromPinFile installs a core.MemoryGate from a pin file.
// Missing / unreadable / tampered pin yields an empty fail-closed gate
// (PIN_MISSING). Evaluate never repairs the file.
func GateFromPinFile(path string) core.Gate {
	g := core.NewMemoryGate()
	if InspectPinFile(path) != PinFileOK {
		return g
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return g
	}
	var p core.Pin
	if err := json.Unmarshal(b, &p); err != nil {
		return g
	}
	_ = g.InstallInitialPin(p)
	return g
}
