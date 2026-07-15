package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuditUserMCPJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(path, []byte(`{"mcpServers": {"pw": {"command": "absent-bin"}}}`), 0644); err != nil {
		t.Fatal(err)
	}
	c := &collector{look: fakeLookPath()}
	c.auditUserMCPJSON(dir)

	if len(c.findings) != 1 || !strings.Contains(c.findings[0].Issue, `server "pw"`) {
		t.Fatalf("expected the ~/.claude/mcp.json server to be evaluated, got %+v", c.findings)
	}
	if c.findings[0].Scope != "user" {
		t.Errorf("scope = %q, want user", c.findings[0].Scope)
	}
}

func TestAuditClaudeJSONPerProjectServers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "claude.json")
	// Top-level server resolves; two per-project servers do not.
	content := `{
		"mcpServers": {"filesystem": {"command": "present-bin"}},
		"projects": {
			"/Users/x/proj-a": {"mcpServers": {"playwright": {"command": "absent-bin"}}},
			"/Users/x/proj-b": {"mcpServers": {"trader": {"command": "absent-bin"}}}
		}
	}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	c := &collector{look: fakeLookPath("present-bin")}
	c.auditClaudeJSON(path)

	var perProject int
	for _, f := range c.findings {
		if strings.Contains(f.Issue, "playwright") || strings.Contains(f.Issue, "trader") {
			perProject++
			if !strings.Contains(f.Issue, "project /Users/x/proj") {
				t.Errorf("per-project finding should name the project: %q", f.Issue)
			}
		}
	}
	if perProject != 2 {
		t.Errorf("expected 2 per-project MCP findings, got %d (%+v)", perProject, c.findings)
	}
	// The resolvable top-level server produces no finding.
	for _, f := range c.findings {
		if strings.Contains(f.Issue, "filesystem") {
			t.Errorf("healthy top-level server should not be flagged: %+v", f)
		}
	}
}
