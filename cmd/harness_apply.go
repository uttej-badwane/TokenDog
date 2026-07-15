package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"tokendog/internal/harness"
)

var harnessApplyYes bool

var harnessApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply the auto-fixable findings from the audit (confirmed per fix, with backups)",
	Long: `Re-runs the audit and applies only the mechanical, reversible fixes:

  - duplicate rules inside a permissions list (removes the extra copies)
  - hook scripts missing their exec bit (chmod +x)

Everything else the audit reports stays report-only — fix it by hand.
Each fix is confirmed individually unless --yes is passed, and every
file is copied to ~/.config/tokendog/harness-backups/<timestamp>/ before
its first change (a manifest.json there maps backups to originals, so a
restore is a plain copy back).`,
	RunE: runHarnessApply,
}

func init() {
	harnessApplyCmd.Flags().BoolVar(&harnessApplyYes, "yes", false, "Apply all fixes without per-fix confirmation")
	harnessCmd.AddCommand(harnessApplyCmd)
}

func runHarnessApply(_ *cobra.Command, _ []string) error {
	report, err := harnessAudit()
	if err != nil {
		return err
	}
	fixes := harness.AutoFixes(report)
	if len(fixes) == 0 {
		fmt.Println("Nothing auto-fixable — run `td harness` for the full report.")
		return nil
	}

	if !harnessApplyYes && !stdinIsTTY() {
		return fmt.Errorf("stdin is not a terminal; pass --yes to apply without confirmation")
	}

	backupDir, err := harness.DefaultBackupDir(time.Now())
	if err != nil {
		return err
	}
	applier := harness.NewApplier(backupDir)
	reader := bufio.NewReader(os.Stdin)

	applied, skipped, failed := 0, 0, 0
	for i, fix := range fixes {
		if !harnessApplyYes {
			fmt.Printf("[%d/%d] %s — apply? [y/N] ", i+1, len(fixes), fix.Description)
			line, _ := reader.ReadString('\n')
			answer := strings.ToLower(strings.TrimSpace(line))
			if answer != "y" && answer != "yes" {
				skipped++
				continue
			}
		}
		if err := applier.Apply(fix); err != nil {
			failed++
			fmt.Printf("✗ %s: %v\n", fix.Description, err)
			continue
		}
		applied++
		fmt.Printf("✓ %s\n", fix.Description)
	}
	if err := applier.Finish(); err != nil {
		return fmt.Errorf("fixes applied but manifest write failed: %w", err)
	}

	fmt.Printf("\n%d applied · %d skipped · %d failed\n", applied, skipped, failed)
	if applied > 0 {
		fmt.Printf("Backups: %s\n", harness.Tildify(backupDir))
	}
	return nil
}

// stdinIsTTY reports whether stdin is an interactive terminal — the
// standard char-device check, no extra dependency.
func stdinIsTTY() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
