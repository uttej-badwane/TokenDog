package harness

import (
	"strings"
	"testing"
)

func TestRuleCovers(t *testing.T) {
	cases := []struct {
		broad, narrow string
		want          bool
	}{
		{"Bash", "Bash(git status)", true},
		{"Bash(*)", "Bash(git status)", true},
		{"Bash(git *)", "Bash(git status)", true},
		{"Bash(git:*)", "Bash(git:status)", true},
		{"Bash(git *)", "Bash(go build)", false},
		{"Bash(git *)", "Bash", false},
		{"Read", "Bash(git status)", false},
		{"Bash(git status)", "Bash(git status --short)", false}, // no trailing *
	}
	for _, c := range cases {
		if got := ruleCovers(parseRule(c.broad), parseRule(c.narrow)); got != c.want {
			t.Errorf("ruleCovers(%q, %q) = %v, want %v", c.broad, c.narrow, got, c.want)
		}
	}
}

func TestAnalyzePermissions(t *testing.T) {
	cfg := map[string]any{
		"permissions": map[string]any{
			"allow": []any{
				"Bash(*)",
				"Bash(git status)", // shadowed by Bash(*)
				"Read(~/notes/*)",
				"Read(~/notes/*)", // exact duplicate
			},
			"deny": []any{"WebFetch"}, // broad deny is fine
		},
	}
	findings := analyzePermissions("/tmp/settings.json", "user", cfg)

	var broad, dupe, shadow int
	for _, f := range findings {
		switch {
		case strings.Contains(f.Issue, "any shell command"):
			broad++
			if f.Severity != SeverityWarning {
				t.Errorf("broad rule severity = %s, want warning", f.Severity)
			}
		case strings.Contains(f.Issue, "appears more than once"):
			dupe++
			if !f.AutoFixable || f.FixID == "" {
				t.Errorf("duplicate finding should be auto-fixable with a FixID: %+v", f)
			}
		case strings.Contains(f.Issue, "already covered by"):
			shadow++
		}
	}
	if broad != 1 || dupe != 1 || shadow != 1 {
		t.Errorf("broad=%d dupe=%d shadow=%d, want 1 each; findings: %+v", broad, dupe, shadow, findings)
	}
}

func TestAnalyzePermissionsCleanAndAbsent(t *testing.T) {
	if got := analyzePermissions("/s.json", "user", map[string]any{}); got != nil {
		t.Errorf("no permissions block should yield nil, got %+v", got)
	}
	cfg := map[string]any{
		"permissions": map[string]any{
			"allow": []any{"Bash(git *)", "Read(~/docs/**)"},
		},
	}
	if got := analyzePermissions("/s.json", "user", cfg); len(got) != 0 {
		t.Errorf("clean permissions should yield no findings, got %+v", got)
	}
}
