package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"tokendog/internal/analytics"
	"tokendog/internal/redact"
)

var (
	purgeBefore string
	purgeRedact bool
	purgeDryRun bool
)

var purgeCmd = &cobra.Command{
	Use:   "purge",
	Short: "Delete or redact analytics history records (for compliance / retention)",
	Long: `Manage ~/.config/tokendog/history.jsonl beyond the automatic 90-day
rotation:

  td purge --before 2026-01-01      # delete records older than this date
  td purge --redact                 # rewrite Command field to scrub secrets
  td purge --before 7d --dry-run    # see what would be removed

Date filters accept YYYY-MM-DD or relative durations (7d/2w/1m/1y), the
same shapes ` + "`td gain --since`" + ` accepts.

Atomic: rewrites via temp + rename so a crash mid-purge can't corrupt
history. The cache directory (~/.config/tokendog/cache/) is NOT touched —
its 1-hour prune handles that.`,
	RunE: runPurge,
}

func init() {
	purgeCmd.Flags().StringVar(&purgeBefore, "before", "", "Delete records on or before this date (YYYY-MM-DD or 7d/2w/1m/1y)")
	purgeCmd.Flags().BoolVar(&purgeRedact, "redact", false, "Rewrite Command field of remaining records to scrub secrets (AWS keys, GH tokens, JWTs, PEM blocks)")
	purgeCmd.Flags().BoolVar(&purgeDryRun, "dry-run", false, "Print counts without modifying anything")
}

func runPurge(_ *cobra.Command, _ []string) error {
	if purgeBefore == "" && !purgeRedact {
		return fmt.Errorf("nothing to do — pass --before <date> and/or --redact")
	}

	cutoff, err := parseDateOrDuration(purgeBefore)
	if err != nil {
		return fmt.Errorf("--before %q: %w", purgeBefore, err)
	}

	records, err := analytics.LoadAll()
	if err != nil {
		return err
	}

	var keep []analytics.Record
	deleted := 0
	redactedCount := 0
	for _, r := range records {
		if !cutoff.IsZero() && !r.Timestamp.After(cutoff) {
			deleted++
			continue
		}
		if purgeRedact {
			if redacted, n := redact.All(r.Command); n > 0 {
				r.Command = redacted
				redactedCount += n
			}
		}
		keep = append(keep, r)
	}

	fmt.Printf("Scanned %d records.\n", len(records))
	if !cutoff.IsZero() {
		fmt.Printf("  Older than %s: %d records to delete\n", cutoff.Format("2006-01-02"), deleted)
	}
	if purgeRedact {
		fmt.Printf("  Secrets redacted across %d field-value matches\n", redactedCount)
	}
	if purgeDryRun {
		fmt.Println("(dry-run; no changes written)")
		return nil
	}
	if deleted == 0 && redactedCount == 0 {
		fmt.Println("Nothing to write.")
		return nil
	}

	if err := writeHistory(keep); err != nil {
		return err
	}
	fmt.Printf("Wrote %d records to history.jsonl.\n", len(keep))
	return nil
}

// writeHistory replaces history.jsonl with the given records, atomically.
// Goes via temp + rename so a crash mid-write can't corrupt the file.
func writeHistory(records []analytics.Record) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, ".config", "tokendog", "history.jsonl")
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	for _, r := range records {
		if err := enc.Encode(r); err != nil {
			f.Close()
			return err
		}
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
