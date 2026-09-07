package adoptmcp

import (
	"testing"

	"github.com/furyheimdall/toolfence-deadbugz-guard/core"
)

func TestProfileFromArgv(t *testing.T) {
	cases := []struct {
		argv []string
		want string
	}{
		{[]string{"mock-mcp-adopt", "filesystem", "/tmp/allowed"}, ProfileFilesystem},
		{[]string{"mock-mcp-adopt", "fetch"}, ProfileFetch},
		{[]string{"mock-mcp-adopt", "github", "--toolsets", "repos"}, ProfileGitHub},
		{[]string{"/ABS/github-mcp-server", "--toolsets", "issues"}, ProfileGitHub},
		{[]string{"uvx", "mcp-server-fetch"}, ProfileFetch},
	}
	for _, tc := range cases {
		if got := ProfileFromArgv(tc.argv); got != tc.want {
			t.Fatalf("ProfileFromArgv(%v)=%s want %s", tc.argv, got, tc.want)
		}
	}
}

func TestParseToolsetsEnvWins(t *testing.T) {
	got := ParseToolsets(
		[]string{"github-mcp-server", "--toolsets", "repos"},
		[]string{"GITHUB_TOOLSETS=repos,issues"},
	)
	if len(got) != 2 || got[0] != "repos" || got[1] != "issues" {
		t.Fatalf("env should win: %v", got)
	}
}

func TestGitHubToolsInventoryChanges(t *testing.T) {
	repos := GitHubTools([]string{"repos"})
	both := GitHubTools([]string{"repos", "issues"})
	if len(both) <= len(repos) {
		t.Fatalf("adding issues must grow inventory: repos=%d both=%d", len(repos), len(both))
	}
	if !hasTool(repos, "get_file_contents") || hasTool(repos, "list_issues") {
		t.Fatalf("repos-only tools: %+v", toolNames(repos))
	}
	if !hasTool(both, "list_issues") {
		t.Fatalf("repos,issues missing list_issues: %+v", toolNames(both))
	}
}

func TestPoisonSameNamesDifferentDesc(t *testing.T) {
	base := FilesystemTools()
	poisoned := Poison(base)
	if poisoned[0].Name != base[0].Name || poisoned[0].Description == base[0].Description {
		t.Fatal("poison must keep the name and rewrite the description")
	}
}

func TestFilesystemAndFetchCatalogs(t *testing.T) {
	fs := FilesystemTools()
	if !hasTool(fs, "read_text_file") || !hasTool(fs, "list_allowed_directories") {
		t.Fatalf("filesystem catalog: %+v", toolNames(fs))
	}
	if !hasTool(FetchTools(), "fetch") {
		t.Fatal("fetch catalog must advertise fetch")
	}
}

func hasTool(tools []core.ToolDef, name string) bool {
	for _, t := range tools {
		if t.Name == name {
			return true
		}
	}
	return false
}

func toolNames(tools []core.ToolDef) []string {
	out := make([]string, len(tools))
	for i, t := range tools {
		out[i] = t.Name
	}
	return out
}
