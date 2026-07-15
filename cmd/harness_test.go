package cmd

import (
	"strings"
	"testing"
	"time"

	"tokendog/internal/harness"
)

func TestRenderHarnessClean(t *testing.T) {
	r := &harness.Report{
		Schema: harness.Schema, GeneratedAt: time.Now(), TDVersion: "test",
		ClaudeHome: "/home/u/.claude",
	}
	out := renderHarness(r)
	if !strings.Contains(out, "✓ No issues found") {
		t.Errorf("clean report should say no issues:\n%s", out)
	}
}

func TestRenderHarnessFindingsAndFooter(t *testing.T) {
	prevSeverity := harnessSeverity
	harnessSeverity = ""
	defer func() { harnessSeverity = prevSeverity }()

	r := &harness.Report{
		Schema: harness.Schema, GeneratedAt: time.Now(), TDVersion: "test",
		ClaudeHome:  "/home/u/.claude",
		ProjectRoot: "/home/u/proj",
		Summary:     harness.Summary{Critical: 1, Warning: 1, AutoFixable: 1, FilesScanned: 5},
		Findings: []harness.Finding{
			{File: "/home/u/.claude/settings.json", Issue: "hook pipes curl to sh", Severity: harness.SeverityCritical,
				Fix: "review before executing", Scope: "user", Dimension: "hooks"},
			{File: "/home/u/.claude/settings.json", Issue: "rule appears twice", Severity: harness.SeverityInfo,
				Fix: "remove the duplicate", Scope: "user", Dimension: "permissions", AutoFixable: true},
		},
	}
	out := renderHarness(r)
	for _, want := range []string{"CRITICAL", "hook pipes curl to sh", "↳ fix:", "td harness apply", "1 auto-fixable"} {
		if !strings.Contains(out, want) {
			t.Errorf("render missing %q:\n%s", want, out)
		}
	}
}

func TestSeverityFloor(t *testing.T) {
	prev := harnessSeverity
	defer func() { harnessSeverity = prev }()

	harnessSeverity = "critical"
	if severityShown(harness.SeverityWarning) || !severityShown(harness.SeverityCritical) {
		t.Error("critical floor should hide warnings")
	}
	harnessSeverity = "warning"
	if severityShown(harness.SeverityInfo) || !severityShown(harness.SeverityWarning) {
		t.Error("warning floor should hide info")
	}
	harnessSeverity = ""
	if !severityShown(harness.SeverityInfo) {
		t.Error("empty floor should show everything")
	}
}
