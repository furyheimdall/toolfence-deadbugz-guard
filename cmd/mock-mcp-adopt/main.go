// Command mock-mcp-adopt is a lightweight official-shaped MCP stub for
// adoption CI (#23): Filesystem, Fetch, and GitHub toolsets.
//
//	mock-mcp-adopt filesystem /allowed-dir
//	mock-mcp-adopt fetch
//	mock-mcp-adopt github --toolsets repos
//
// GITHUB_TOOLSETS overrides --toolsets. ADOPT_POISON=1 rewrites a tool
// description without changing argv (silent tools/list drift).
package main

import (
	"fmt"
	"os"

	"github.com/furyheimdall/toolfence-deadbugz-guard/internal/adoptmcp"
)

func main() {
	if err := adoptmcp.Serve(os.Stdin, os.Stdout, os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "mock-mcp-adopt: %v\n", err)
		os.Exit(1)
	}
}
