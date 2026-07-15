// Package harness audits the user's Claude Code setup: it inventories
// every config under ~/.claude, ~/.claude.json, and the active project's
// .claude/ dir, runs deterministic offline checks (no LLM, no network),
// and produces a ranked findings report.
//
// The audit itself is strictly read-only. The only mutations live in
// apply.go, are limited to a tiny allow-list of mechanical fixes, and are
// invoked exclusively through `td harness apply` after explicit user
// approval, with a backup of every touched file.
//
// Why a separate package: every analyzer is a pure function over parsed
// content (with injectable LookPath / path roots), so the whole audit is
// testable against a synthetic tree in t.TempDir() — same philosophy as
// internal/mcpconfig.
package harness

import (
	"os/exec"
	"path/filepath"
	"time"
)

// Severity buckets for findings. Ordered: critical > warning > info.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityWarning  Severity = "warning"
	SeverityInfo     Severity = "info"
)

// rank orders severities for sorting; lower sorts first.
func (s Severity) rank() int {
	switch s {
	case SeverityCritical:
		return 0
	case SeverityWarning:
		return 1
	default:
		return 2
	}
}

// Options configures a harness run. Zero values resolve to the real
// environment (real ~/.claude, exec.LookPath, time.Now); tests inject
// temp roots and fakes.
type Options struct {
	ClaudeHome  string // "" → ClaudeHome() (~/.claude, TD_CLAUDE_HOME override)
	ClaudeJSON  string // "" → ClaudeJSONPath() (~/.claude.json, TD_CLAUDE_JSON override)
	ProjectRoot string // "" → no project scope (callers auto-detect via FindProjectRoot)
	Version     string // td version stamped into the report

	LookPath func(string) (string, error) // nil → exec.LookPath
	Now      func() time.Time             // nil → time.Now
}

func (o *Options) fill() error {
	if o.LookPath == nil {
		o.LookPath = exec.LookPath
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.ClaudeHome == "" {
		home, err := ClaudeHome()
		if err != nil {
			return err
		}
		o.ClaudeHome = home
	}
	if o.ClaudeJSON == "" {
		path, err := ClaudeJSONPath()
		if err != nil {
			return err
		}
		o.ClaudeJSON = path
	}
	return nil
}

// Run executes the full audit and returns the report. A missing
// ~/.claude (or any individual file) is a valid, clean state — never an
// error. Errors are reserved for environment failures (e.g. no home dir).
func Run(opts Options) (*Report, error) {
	if err := opts.fill(); err != nil {
		return nil, err
	}

	c := &collector{look: opts.LookPath}

	// User scope: ~/.claude and its CLAUDE.md.
	userSettings := c.auditScope("user", opts.ClaudeHome, opts.ClaudeHome)
	c.auditKeybindings(opts.ClaudeHome)
	c.auditMemoryDirs(opts.ClaudeHome)

	// Project scope: <root>/.claude plus root-level CLAUDE.md and .mcp.json.
	var projSettings map[string]any
	if opts.ProjectRoot != "" {
		projSettings = c.auditScope("project", filepath.Join(opts.ProjectRoot, ".claude"), opts.ProjectRoot)
		c.auditProjectMCP(opts.ProjectRoot)
	}

	// Global MCP sources: ~/.claude/mcp.json, ~/.claude.json (global +
	// per-project), and Claude Desktop's config.
	c.auditUserMCPJSON(opts.ClaudeHome)
	c.auditClaudeJSON(opts.ClaudeJSON)
	c.auditDesktopConfig()

	// Cross-cutting: same scalar key set in both settings scopes.
	c.findings = append(c.findings, analyzeCrossScope(userSettings, projSettings, c.userSettingsPath, c.projSettingsPath)...)

	// Name collisions between user and project commands/skills.
	c.findings = append(c.findings, c.collisionFindings()...)

	return buildReport(opts, c), nil
}
