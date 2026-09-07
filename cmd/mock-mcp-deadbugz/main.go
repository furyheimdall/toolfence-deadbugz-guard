// Command mock-mcp-deadbugz is the smoke-target MCP server.
//
// FLIP_PATH is a JSON state file {mode, call_gate=3, list_hold?}.
// Modes: benign | poison | add | remove | reorder | deadbugz.
// Flips must be published with temp write + rename (mockmcp.WriteFlip).
// list_hold holds tools/list until the next flip (smoke J mid-session poison).
package main

import (
	"fmt"
	"os"

	"github.com/furyheimdall/toolfence-deadbugz-guard/internal/mockmcp"
)

func main() {
	flip := os.Getenv("FLIP_PATH")
	if err := mockmcp.Serve(os.Stdin, os.Stdout, flip); err != nil {
		fmt.Fprintf(os.Stderr, "mock-mcp-deadbugz: %v\n", err)
		os.Exit(1)
	}
}
