package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"tokendog/internal/compress"
	"tokendog/internal/tokenizer"
)

var (
	compressDryRun  bool
	compressNoBack  bool
	compressInPlace bool
)

var compressCmd = &cobra.Command{
	Use:   "compress [file...]",
	Short: "Compress CLAUDE.md / memory files to cut input tokens",
	Long: `Rewrite natural-language memory files in terse form to reduce the input
tokens billed on every Claude Code session.

Strips filler words, articles, pleasantries, and hedging phrases from prose
while leaving code blocks, inline code, URLs, file paths, and identifiers
byte-for-byte unchanged. The output is always ≤ the input.

Default targets when no file is given:
  ./CLAUDE.md
  ~/.claude/CLAUDE.md
  ~/.claude/projects/**/*.md   (heuristic: files with "memory" in the path)

A .original.md backup is written before the file is overwritten unless
--no-backup is set. Run with --dry-run to preview savings without writing.`,
	Example: `  td compress                           # auto-discover CLAUDE.md files
  td compress ./CLAUDE.md                # specific file
  td compress --dry-run ~/.claude/CLAUDE.md
  td compress --no-backup NOTES.md`,
	RunE: runCompress,
}

func init() {
	compressCmd.Flags().BoolVar(&compressDryRun, "dry-run", false, "Print savings without writing any files")
	compressCmd.Flags().BoolVar(&compressNoBack, "no-backup", false, "Overwrite in place without creating a .original.md backup")
	_ = compressNoBack // silence unused warning; used in runCompress
}

func runCompress(_ *cobra.Command, args []string) error {
	targets, err := resolveCompressTargets(args)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		fmt.Println("No target files found. Pass a file path or create CLAUDE.md in the current directory.")
		return nil
	}

	totalBefore, totalAfter := 0, 0
	for _, path := range targets {
		before, after, err := compressOne(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %s: %v\n", path, err)
			continue
		}
		totalBefore += before
		totalAfter += after
	}

	if len(targets) > 1 {
		fmt.Printf("\n%-26s %d → %d tokens  (%+.0f%%)\n",
			"Total:", tokenizer.Count(strings.Repeat("x", totalBefore)),
			tokenizer.Count(strings.Repeat("x", totalAfter)),
			savingPct(totalBefore, totalAfter))
	}
	return nil
}

func compressOne(path string) (before, after int, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, err
	}
	original := string(raw)
	compressed, changed := compress.CompressFile(original)

	tokBefore := tokenizer.Count(original)
	tokAfter := tokenizer.Count(compressed)

	rel := path
	if home, _ := os.UserHomeDir(); home != "" {
		if r, e := filepath.Rel(home, path); e == nil && !strings.HasPrefix(r, "..") {
			rel = "~/" + r
		}
	}

	if !changed {
		fmt.Printf("  %-48s  already terse  (%d tokens)\n", rel, tokBefore)
		return tokBefore, tokBefore, nil
	}

	pct := savingPct(tokBefore, tokAfter)
	if compressDryRun {
		fmt.Printf("  %-48s  %d → %d tokens  (%+.0f%%)  [dry-run]\n", rel, tokBefore, tokAfter, pct)
		return tokBefore, tokAfter, nil
	}

	// Write backup unless suppressed.
	if !compressNoBack {
		ext := filepath.Ext(path)
		base := strings.TrimSuffix(path, ext)
		backupPath := base + ".original" + ext
		if _, statErr := os.Stat(backupPath); statErr == nil {
			return 0, 0, fmt.Errorf("backup %s already exists — remove it to re-compress", backupPath)
		}
		if err := os.WriteFile(backupPath, raw, 0644); err != nil {
			return 0, 0, fmt.Errorf("writing backup: %w", err)
		}
	}

	if err := os.WriteFile(path, []byte(compressed), 0644); err != nil {
		return 0, 0, fmt.Errorf("writing compressed file: %w", err)
	}

	fmt.Printf("  %-48s  %d → %d tokens  (%+.0f%%)\n", rel, tokBefore, tokAfter, pct)
	return tokBefore, tokAfter, nil
}

// resolveCompressTargets returns the files to compress. If args are given,
// they're used directly. Otherwise, the command auto-discovers CLAUDE.md
// files in the cwd and home directory.
func resolveCompressTargets(args []string) ([]string, error) {
	if len(args) > 0 {
		var out []string
		for _, a := range args {
			abs, err := filepath.Abs(a)
			if err != nil {
				return nil, err
			}
			if _, err := os.Stat(abs); err != nil {
				return nil, fmt.Errorf("%s: %w", a, err)
			}
			out = append(out, abs)
		}
		return out, nil
	}

	// Auto-discovery.
	seen := map[string]bool{}
	var targets []string

	candidates := []string{"./CLAUDE.md"}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(home, ".claude", "CLAUDE.md"),
		)
		// Memory files under ~/.claude/projects/*/memory/
		if matches, err := filepath.Glob(filepath.Join(home, ".claude", "projects", "*", "memory", "*.md")); err == nil {
			candidates = append(candidates, matches...)
		}
	}

	for _, c := range candidates {
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		if seen[abs] {
			continue
		}
		if _, err := os.Stat(abs); err != nil {
			continue
		}
		// Skip .original.md backups so we don't re-compress our own backups.
		if strings.HasSuffix(abs, ".original.md") {
			continue
		}
		seen[abs] = true
		targets = append(targets, abs)
	}
	return targets, nil
}

func savingPct(before, after int) float64 {
	if before == 0 {
		return 0
	}
	return float64(after-before) / float64(before) * 100
}
