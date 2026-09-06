// Package mockmcp is the smoke-target MCP server (FLIP_PATH modes).
package mockmcp

import (
	"encoding/json"

	"github.com/furyheimdall/toolfence-deadbugz-guard/core"
)

// Modes accepted via FLIP_PATH or MODE (epic #1 fixtures).
const (
	ModeBenign   = "benign"
	ModePoison   = "poison"
	ModeAdd      = "add"
	ModeRemove   = "remove"
	ModeReorder  = "reorder"
	ModeDeadbugz = "deadbugz"
)

func schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}

func tool(name, desc string) core.ToolDef {
	return core.ToolDef{Name: name, Description: desc, InputSchema: schema()}
}

// Tools returns the listing for a fixture mode.
func Tools(mode string) []core.ToolDef {
	alpha := tool("alpha", "First tool")
	bravo := tool("bravo", "Second tool")
	switch mode {
	case ModeReorder:
		return []core.ToolDef{bravo, alpha}
	case ModePoison, ModeDeadbugz:
		return []core.ToolDef{tool("alpha", "POISONED first tool"), bravo}
	case ModeAdd:
		return []core.ToolDef{alpha, bravo, tool("charlie", "Added tool")}
	case ModeRemove:
		return []core.ToolDef{alpha}
	default:
		return []core.ToolDef{alpha, bravo}
	}
}

// NormalizeMode maps empty / unknown to benign.
func NormalizeMode(mode string) string {
	switch mode {
	case ModeBenign, ModePoison, ModeAdd, ModeRemove, ModeReorder, ModeDeadbugz:
		return mode
	default:
		return ModeBenign
	}
}
