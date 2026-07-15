package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"tokendog/internal/harness"
)

var (
	harnessJSON      bool
	harnessProject   string
	harnessSeverity  string
	harnessInventory bool
)

var harnessCmd = &cobra.Command{
	Use:   "harness",
	Short: "Audit your Claude Code setup (settings, permissions, hooks, memory, agents, MCP)",
	Long: `Code Harness: inventory and audit every Claude Code config —
~/.claude (settings, CLAUDE.md + @imports, agents, commands, skills,
memory, keybindings), ~/.claude.json MCP servers, the active project's
.claude/ and .mcp.json, and Claude Desktop's MCP config.

Checks are deterministic and fully offline: no LLM, no network. The
audit is strictly read-only; findings are ranked critical > warning >
info with a concrete fix for each. The handful of mechanical fixes
(duplicate permission rules, hook scripts missing their exec bit) can be
applied — after per-fix confirmation and with backups — via
` + "`td harness apply`" + `.

--json emits a stable, versioned contract used by the macOS menu-bar app.`,
	RunE: runHarness,
}

func init() {
	// --project is persistent so `td harness apply` audits the same tree.
	harnessCmd.PersistentFlags().StringVar(&harnessProject, "project", "", "Project root to audit (default: auto-detect from the working directory)")
	harnessCmd.Flags().BoolVar(&harnessJSON, "json", false, "Emit the machine-readable report instead of a table")
	harnessCmd.Flags().StringVar(&harnessSeverity, "severity", "", "Only show findings at or above this severity (critical|warning|info)")
	harnessCmd.Flags().BoolVar(&harnessInventory, "inventory", false, "Also list every inventoried file in the human report")
}

func runHarness(_ *cobra.Command, _ []string) error {
	report, err := harnessAudit()
	if err != nil {
		return err
	}
	if harnessJSON {
		return emitJSON(report)
	}
	fmt.Print(renderHarness(report))
	return nil
}

// harnessAudit resolves the project root and runs the audit. Shared by
// `td harness` and `td harness apply`.
func harnessAudit() (*harness.Report, error) {
	projectRoot := harnessProject
	if projectRoot == "" {
		if cwd, err := os.Getwd(); err == nil {
			projectRoot = harness.FindProjectRoot(cwd)
		}
	}
	return harness.Run(harness.Options{
		ProjectRoot: projectRoot,
		Version:     Version,
	})
}

// severityShown applies the --severity floor. Empty flag shows all.
func severityShown(s harness.Severity) bool {
	switch harnessSeverity {
	case "critical":
		return s == harness.SeverityCritical
	case "warning":
		return s == harness.SeverityCritical || s == harness.SeverityWarning
	default:
		return true
	}
}

func renderHarness(r *harness.Report) string {
	var b strings.Builder
	rule := strings.Repeat("─", 78)

	fmt.Fprintf(&b, "Code Harness — Claude Code setup audit (td %s)\n", r.TDVersion)
	b.WriteString(strings.Repeat("═", 78) + "\n")
	fmt.Fprintf(&b, "  %-10s %s\n", "User:", harness.Tildify(r.ClaudeHome))
	if r.ProjectRoot != "" {
		fmt.Fprintf(&b, "  %-10s %s\n", "Project:", harness.Tildify(r.ProjectRoot))
	} else {
		fmt.Fprintf(&b, "  %-10s %s\n", "Project:", "(none detected — run from a project dir or pass --project)")
	}
	s := r.Summary
	fmt.Fprintf(&b, "  %-10s %d files · %d critical · %d warning · %d info · %d auto-fixable\n",
		"Scanned:", s.FilesScanned, s.Critical, s.Warning, s.Info, s.AutoFixable)

	if harnessInventory {
		b.WriteString("\nINVENTORY\n" + rule + "\n")
		for _, it := range r.Inventory {
			status := "ok"
			if !it.Parses {
				status = "PARSE ERROR"
			}
			fmt.Fprintf(&b, "  %-44s %-12s %-8s %7s  %s\n",
				truncPath(harness.Tildify(it.Path), 44), it.Kind, it.Scope, humanSize(it.SizeBytes), status)
		}
	}

	shown := 0
	b.WriteString("\nFINDINGS\n" + rule + "\n")
	for _, f := range r.Findings {
		if !severityShown(f.Severity) {
			continue
		}
		shown++
		tag := strings.ToUpper(string(f.Severity))
		if f.AutoFixable {
			tag += " ·fix"
		}
		fmt.Fprintf(&b, "%-14s %s/%s  %s\n", tag, f.Scope, f.Dimension, harness.Tildify(f.File))
		fmt.Fprintf(&b, "               %s\n", f.Issue)
		fmt.Fprintf(&b, "               ↳ fix: %s\n\n", f.Fix)
	}
	if shown == 0 {
		if len(r.Findings) == 0 {
			b.WriteString("✓ No issues found — your Claude Code setup is clean.\n")
		} else {
			fmt.Fprintf(&b, "✓ Nothing at or above --severity %s (%d findings below the floor).\n", harnessSeverity, len(r.Findings))
		}
	}
	if s.AutoFixable > 0 {
		b.WriteString(rule + "\n")
		fmt.Fprintf(&b, "Run `td harness apply` to fix the %d auto-fixable finding(s) — each one\nis confirmed individually and every touched file is backed up first.\n", s.AutoFixable)
	}
	return b.String()
}

func truncPath(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n+1:]
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1fM", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1fK", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}
