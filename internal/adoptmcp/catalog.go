// Package adoptmcp is a lightweight official-shaped MCP inventory for
// adoption CI (#23). It is not a product server and does not expand smoke G–J.
//
// Profiles match the Cursor attach docs (Filesystem + Fetch) and GitHub
// official MCP toolsets (`--toolsets` / GITHUB_TOOLSETS). Changing toolsets
// is an intentional inventory change; ADOPT_POISON mutates tools/list under
// the same argv so the wrap must emit tools_list_drift.
package adoptmcp

import (
	"encoding/json"
	"strings"

	"github.com/furyheimdall/toolfence-deadbugz-guard/core"
)

// Profiles used by mock-mcp-adopt and verify scripts.
const (
	ProfileFilesystem = "filesystem"
	ProfileFetch      = "fetch"
	ProfileGitHub     = "github"
)

func schema(props string) json.RawMessage {
	if props == "" {
		return json.RawMessage(`{"type":"object","properties":{}}`)
	}
	return json.RawMessage(`{"type":"object","properties":` + props + `}`)
}

func tool(name, desc, props string) core.ToolDef {
	return core.ToolDef{Name: name, Description: desc, InputSchema: schema(props)}
}

// FilesystemTools is the official-shaped @modelcontextprotocol/server-filesystem
// listing used by examples/cursor.mcp.json and the plugin-guide 60s CTA.
func FilesystemTools() []core.ToolDef {
	return []core.ToolDef{
		tool("read_text_file", "Read the complete contents of a file as text.", `{"path":{"type":"string"}}`),
		tool("read_media_file", "Read an image or audio file.", `{"path":{"type":"string"}}`),
		tool("read_multiple_files", "Read multiple files at once.", `{"paths":{"type":"array"}}`),
		tool("write_file", "Create a new file or overwrite an existing one.", `{"path":{"type":"string"},"content":{"type":"string"}}`),
		tool("edit_file", "Make line-based edits to a text file.", `{"path":{"type":"string"}}`),
		tool("create_directory", "Create a new directory.", `{"path":{"type":"string"}}`),
		tool("list_directory", "List files and directories.", `{"path":{"type":"string"}}`),
		tool("list_directory_with_sizes", "List a directory with file sizes.", `{"path":{"type":"string"}}`),
		tool("directory_tree", "Get a recursive tree view of files.", `{"path":{"type":"string"}}`),
		tool("move_file", "Move or rename a file or directory.", `{"source":{"type":"string"},"destination":{"type":"string"}}`),
		tool("search_files", "Recursively search for files.", `{"path":{"type":"string"},"pattern":{"type":"string"}}`),
		tool("get_file_info", "Retrieve file metadata.", `{"path":{"type":"string"}}`),
		tool("list_allowed_directories", "List directories the server can access.", `{}`),
	}
}

// FetchTools is the official-shaped uvx mcp-server-fetch listing.
func FetchTools() []core.ToolDef {
	return []core.ToolDef{
		tool("fetch", "Fetch a URL and extract its contents as markdown.", `{"url":{"type":"string"},"max_length":{"type":"number"},"raw":{"type":"boolean"}}`),
	}
}

// GitHubToolsets is a representative subset of github-mcp-server groups.
// Full github-mcp-server is too heavy for CI; this stub still changes
// advertised tools when --toolsets / GITHUB_TOOLSETS changes so the wrap
// hits the real config_fingerprint path.
var GitHubToolsets = map[string][]core.ToolDef{
	"context": {
		tool("get_me", "Get details of the authenticated GitHub user.", `{}`),
	},
	"repos": {
		tool("get_file_contents", "Get contents of a file or directory in a repository.", `{"owner":{"type":"string"},"repo":{"type":"string"},"path":{"type":"string"}}`),
		tool("list_commits", "Get a list of commits of a branch in a repository.", `{"owner":{"type":"string"},"repo":{"type":"string"}}`),
	},
	"issues": {
		tool("list_issues", "List issues in a GitHub repository.", `{"owner":{"type":"string"},"repo":{"type":"string"}}`),
		tool("issue_write", "Create or update an issue in a GitHub repository.", `{"owner":{"type":"string"},"repo":{"type":"string"},"title":{"type":"string"}}`),
	},
	"pull_requests": {
		tool("create_pull_request", "Create a pull request in a GitHub repository.", `{"owner":{"type":"string"},"repo":{"type":"string"},"title":{"type":"string"}}`),
	},
	"users": {
		tool("search_users", "Find GitHub users by username or email.", `{"q":{"type":"string"}}`),
	},
	"actions": {
		tool("list_workflow_runs", "List GitHub Actions workflow runs.", `{"owner":{"type":"string"},"repo":{"type":"string"}}`),
	},
}

// ParseToolsets reads official GitHub MCP inventory flags/env.
// GITHUB_TOOLSETS / GITHUB_TOOLS take precedence over --toolsets / --tools
// (same precedence as github-mcp-server).
func ParseToolsets(argv []string, environ []string) []string {
	fromEnv := ""
	for _, kv := range environ {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		switch k {
		case "GITHUB_TOOLSETS", "GITHUB_TOOLS", "MCP_TOOLSETS", "MCP_TOOLS":
			if strings.TrimSpace(v) != "" {
				fromEnv = v
			}
		}
	}
	if fromEnv != "" {
		return splitCSV(fromEnv)
	}
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		flag, val, inline := splitFlag(arg)
		if flag != "--toolsets" && flag != "--tools" {
			continue
		}
		if inline {
			return splitCSV(val)
		}
		if i+1 < len(argv) && !strings.HasPrefix(argv[i+1], "-") {
			return splitCSV(argv[i+1])
		}
	}
	return []string{"context", "repos"}
}

// GitHubTools returns the union of selected toolsets (stable name order).
func GitHubTools(toolsets []string) []core.ToolDef {
	seen := map[string]struct{}{}
	var out []core.ToolDef
	for _, ts := range toolsets {
		ts = strings.TrimSpace(strings.ToLower(ts))
		if ts == "" || ts == "default" {
			ts = "context"
		}
		if ts == "all" {
			for name, tools := range GitHubToolsets {
				if _, ok := seen[name]; ok {
					continue
				}
				seen[name] = struct{}{}
				out = append(out, tools...)
			}
			continue
		}
		if _, ok := seen[ts]; ok {
			continue
		}
		tools, ok := GitHubToolsets[ts]
		if !ok {
			continue
		}
		seen[ts] = struct{}{}
		out = append(out, tools...)
	}
	if len(out) == 0 {
		return GitHubTools([]string{"context", "repos"})
	}
	return out
}

// ToolsForProfile returns the live catalog for a wrap profile.
func ToolsForProfile(profile string, argv []string, environ []string) []core.ToolDef {
	var tools []core.ToolDef
	switch NormalizeProfile(profile) {
	case ProfileFetch:
		tools = FetchTools()
	case ProfileGitHub:
		tools = GitHubTools(ParseToolsets(argv, environ))
	default:
		tools = FilesystemTools()
	}
	if poisonEnv(environ) {
		return Poison(tools)
	}
	return tools
}

// Poison rewrites the first tool description (silent tools/list drift).
func Poison(tools []core.ToolDef) []core.ToolDef {
	if len(tools) == 0 {
		return tools
	}
	out := append([]core.ToolDef(nil), tools...)
	out[0].Description = "POISONED " + out[0].Description
	return out
}

// NormalizeProfile maps empty / unknown to filesystem (Cursor CTA default).
func NormalizeProfile(profile string) string {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case ProfileFetch, "mcp-server-fetch":
		return ProfileFetch
	case ProfileGitHub, "github-mcp-server", "github_mcp":
		return ProfileGitHub
	default:
		return ProfileFilesystem
	}
}

// ProfileFromArgv treats argv[0] basename or the first non-flag token as
// the profile. `mock-mcp-adopt github --toolsets repos` and a binary
// named github-mcp-server both resolve to github.
func ProfileFromArgv(argv []string) string {
	if len(argv) == 0 {
		return ProfileFilesystem
	}
	base := argv[0]
	if i := strings.LastIndexAny(base, `/\`); i >= 0 {
		base = base[i+1:]
	}
	if looksLikeProfile(base) {
		return NormalizeProfile(base)
	}
	for _, a := range argv[1:] {
		if strings.HasPrefix(a, "-") {
			continue
		}
		return NormalizeProfile(a)
	}
	return ProfileFilesystem
}

func looksLikeProfile(s string) bool {
	switch strings.ToLower(s) {
	case ProfileFilesystem, ProfileFetch, ProfileGitHub,
		"mcp-server-fetch", "github-mcp-server", "github_mcp",
		"server-filesystem":
		return true
	}
	return false
}

func poisonEnv(environ []string) bool {
	for _, kv := range environ {
		k, v, ok := strings.Cut(kv, "=")
		if ok && k == "ADOPT_POISON" && (v == "1" || strings.EqualFold(v, "true")) {
			return true
		}
	}
	return false
}

func splitFlag(s string) (flag, val string, inline bool) {
	if !strings.HasPrefix(s, "--") {
		return "", "", false
	}
	if i := strings.IndexByte(s, '='); i >= 0 {
		return s[:i], s[i+1:], true
	}
	return s, "", false
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToLower(p))
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}
