package harness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeTree creates files relative to root from a map of path→content.
func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRunFullTree(t *testing.T) {
	tmp := t.TempDir()
	claudeHome := filepath.Join(tmp, "dot-claude")
	project := filepath.Join(tmp, "project")
	t.Setenv("TD_CLAUDE_DESKTOP_CONFIG", filepath.Join(tmp, "desktop.json"))

	writeTree(t, claudeHome, map[string]string{
		"settings.json": `{
			"model": "opus",
			"permissions": {"allow": ["Bash(*)", "Bash(git *)", "Bash(git *)"]},
			"unknownKey": true
		}`,
		"CLAUDE.md":           "# Rules\n@missing-import.md\n",
		"agents/reviewer.md":  "---\nname: reviewer\ndescription: Reviews pull requests for style and correctness issues\ntools: Read\n---\n",
		"commands/deploy.md":  "Deploy the app",
		"skills/fmt/SKILL.md": "---\nname: fmt\ndescription: Formats the whole repo with the project formatter\n---\n",
		"memory/MEMORY.md":    "- [A](a.md) — hook\n",
		"memory/a.md":         "fact",
		"memory/orphan.md":    "unindexed",
		"keybindings.json":    `{"bindings": []}`,
	})
	writeTree(t, project, map[string]string{
		".claude/settings.json":      `{"model": "sonnet"}`,
		"CLAUDE.md":                  "# Project rules\n",
		".claude/commands/deploy.md": "Project deploy",
		".mcp.json":                  `{"mcpServers": {"gone": {"command": "absent-bin"}}}`,
	})
	claudeJSON := filepath.Join(tmp, "claude.json")
	if err := os.WriteFile(claudeJSON, []byte(`{"mcpServers": {"ok": {"command": "present-bin"}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	report, err := Run(Options{
		ClaudeHome:  claudeHome,
		ClaudeJSON:  claudeJSON,
		ProjectRoot: project,
		Version:     "test",
		LookPath:    fakeLookPath("present-bin"),
		Now:         func() time.Time { return time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}

	wantIssues := map[string]bool{
		"broad allow":       false,
		"duplicate rule":    false,
		"unknown key":       false,
		"broken import":     false,
		"memory orphan":     false,
		"mcp binary":        false,
		"command collision": false,
		"cross-scope model": false,
	}
	for _, f := range report.Findings {
		switch {
		case contains(f.Issue, "any shell command"):
			wantIssues["broad allow"] = true
		case contains(f.Issue, "appears more than once"):
			wantIssues["duplicate rule"] = true
			if !f.AutoFixable {
				t.Errorf("duplicate rule should be auto-fixable: %+v", f)
			}
		case contains(f.Issue, `"unknownKey"`):
			wantIssues["unknown key"] = true
		case contains(f.Issue, "missing-import.md"):
			wantIssues["broken import"] = true
		case contains(f.Issue, "not listed in MEMORY.md"):
			wantIssues["memory orphan"] = true
		case contains(f.Issue, `"absent-bin"`) || contains(f.Issue, `server "gone"`):
			wantIssues["mcp binary"] = true
		case contains(f.Issue, `command "deploy" shadows`):
			wantIssues["command collision"] = true
		case contains(f.Issue, `"model" is set in both scopes`):
			wantIssues["cross-scope model"] = true
		}
	}
	for name, seen := range wantIssues {
		if !seen {
			t.Errorf("expected finding %q missing; findings:\n%s", name, dumpFindings(report.Findings))
		}
	}

	// Findings sorted: severities never regress.
	lastRank := -1
	for _, f := range report.Findings {
		if r := f.Severity.rank(); r < lastRank {
			t.Errorf("findings not sorted by severity: %+v", report.Findings)
			break
		} else {
			lastRank = r
		}
	}

	// Summary consistent with findings + inventory.
	s := report.Summary
	if s.Critical+s.Warning+s.Info != len(report.Findings) {
		t.Errorf("summary counts %d+%d+%d != %d findings", s.Critical, s.Warning, s.Info, len(report.Findings))
	}
	if s.FilesScanned != len(report.Inventory) || s.FilesScanned == 0 {
		t.Errorf("FilesScanned=%d inventory=%d", s.FilesScanned, len(report.Inventory))
	}

	// JSON round-trip: contract fields survive.
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back["schema"].(float64) != Schema || back["generated_at"] != "2026-07-14T12:00:00Z" {
		t.Errorf("schema/generated_at wrong: %v %v", back["schema"], back["generated_at"])
	}
}

func TestRunMissingEverything(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TD_CLAUDE_DESKTOP_CONFIG", filepath.Join(tmp, "nope.json"))
	report, err := Run(Options{
		ClaudeHome: filepath.Join(tmp, "no-claude"),
		ClaudeJSON: filepath.Join(tmp, "no-claude.json"),
		Version:    "test",
		LookPath:   fakeLookPath(),
	})
	if err != nil {
		t.Fatalf("missing ~/.claude must be a clean state, got error %v", err)
	}
	if len(report.Findings) != 0 || len(report.Inventory) != 0 {
		t.Errorf("expected empty report, got %+v", report)
	}
}

func TestFindProjectRoot(t *testing.T) {
	tmp := t.TempDir()
	nested := filepath.Join(tmp, "repo", "src", "deep")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	if got := FindProjectRoot(nested); got != "" {
		t.Errorf("no markers: got %q, want empty", got)
	}
	if err := os.MkdirAll(filepath.Join(tmp, "repo", ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if got := FindProjectRoot(nested); got != filepath.Join(tmp, "repo") {
		t.Errorf("git root: got %q", got)
	}
	// A nearer .claude wins over the outer .git.
	if err := os.MkdirAll(filepath.Join(tmp, "repo", "src", ".claude"), 0755); err != nil {
		t.Fatal(err)
	}
	if got := FindProjectRoot(nested); got != filepath.Join(tmp, "repo", "src") {
		t.Errorf("nearest .claude: got %q", got)
	}
}

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

func dumpFindings(findings []Finding) string {
	out := ""
	for _, f := range findings {
		out += "  [" + string(f.Severity) + "] " + f.File + ": " + f.Issue + "\n"
	}
	return out
}
