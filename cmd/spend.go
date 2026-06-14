package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"tokendog/internal/spend"
)

var spendJSON bool

var spendCmd = &cobra.Command{
	Use:   "spend",
	Short: "Show Claude API spend (today / month / lifetime)",
	Long: `Compute Claude API spend from Claude Code's local usage logs
(~/.claude/projects/**/*.jsonl), priced natively with TokenDog's per-model
rates — no ccusage or network access required.

The savings TokenDog clawed back are shown alongside. --json emits a stable,
versioned contract used by the macOS menu-bar app (and pipeable into anything
else).`,
	RunE: runSpend,
}

func init() {
	spendCmd.Flags().BoolVar(&spendJSON, "json", false, "Emit the machine-readable menu-bar contract instead of a table")
}

func runSpend(_ *cobra.Command, _ []string) error {
	rep, err := spend.Compute(Version)
	if err != nil {
		return err
	}
	if spendJSON {
		return emitJSON(rep)
	}
	fmt.Print(renderSpend(rep))
	return nil
}

func renderSpend(rep spend.Report) string {
	var b strings.Builder
	dash := strings.Repeat("─", 40)

	b.WriteString("Claude API spend\n")
	b.WriteString(dash + "\n")
	if rep.Spend.Available {
		fmt.Fprintf(&b, "  %-18s $%.2f\n", "Today:", rep.Spend.Today)
		fmt.Fprintf(&b, "  %-18s $%.2f\n", "This month:", rep.Spend.Month)
		fmt.Fprintf(&b, "  %-18s $%.2f\n", "Lifetime:", rep.Spend.Lifetime)
	} else {
		b.WriteString("  (no Claude usage logs found in ~/.claude/projects)\n")
	}
	b.WriteString(dash + "\n")
	fmt.Fprintf(&b, "  %-18s $%.4f\n", "TD saved today:", rep.Saved.Today)
	fmt.Fprintf(&b, "  %-18s $%.4f\n", "TD saved lifetime:", rep.Saved.Lifetime)
	if rep.SharePct > 0 {
		fmt.Fprintf(&b, "  %-18s %.1f%%\n", "TD share of bill:", rep.SharePct)
	}
	b.WriteString(dash + "\n")
	return b.String()
}
