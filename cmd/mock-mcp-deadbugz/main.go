// Command mock-mcp-deadbugz is the smoke-target MCP server.
//
// Mode is read from FLIP_PATH (atomic file) or MODE:
// benign | poison | add | remove | reorder | deadbugz
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
