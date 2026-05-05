package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"tokendog/internal/analytics"
	"tokendog/internal/calibration"
	"tokendog/internal/transcript"
)

var (
	gainHistory bool
	gainSession string
)

var gainCmd = &cobra.Command{
	Use:   "gain",
	Short: "Show token savings summary",
	RunE:  runGain,
}

func init() {
	gainCmd.Flags().BoolVar(&gainHistory, "history", false, "Show recent command history")
	gainCmd.Flags().StringVar(&gainSession, "session", "", "Show savings for a specific session id (or 'current' for the active session)")
}

func runGain(_ *cobra.Command, _ []string) error {
	records, err := analytics.LoadAll()
	if err != nil {
		return err
	}

	// --session: filter records and cross-reference the transcript so users
	// see savings as a fraction of actual session consumption.
	if gainSession != "" {
		return runGainSession(records)
	}

	// Default lifetime view, with calibration multiplier applied if we have
	// enough samples. Recompute on every invocation — it's cheap and avoids
	// stale ratios after the user changes their workflow.
	snap, _ := calibration.Recompute(records)
	if snap != nil {
		_ = calibration.Save(snap)
	}
	fmt.Print(analytics.RenderGain(records, gainHistory, snap))
	return nil
}

// runGainSession renders the per-session view: TD's savings (cl100k) +
// Anthropic's actual token consumption from the transcript JSONL. This is
// the "did td help me on this session?" answer.
func runGainSession(records []analytics.Record) error {
	target := gainSession
	if target == "current" {
		target = os.Getenv("TD_SESSION_ID")
		if target == "" {
			return fmt.Errorf("--session=current requires TD_SESSION_ID in env (run via Claude Code hook)")
		}
	}

	var sessionRecords []analytics.Record
	transcriptPath := ""
	for _, r := range records {
		if r.SessionID == target {
			sessionRecords = append(sessionRecords, r)
			if transcriptPath == "" && r.TranscriptPath != "" {
				transcriptPath = r.TranscriptPath
			}
		}
	}
	if len(sessionRecords) == 0 {
		return fmt.Errorf("no td records found for session %q (sessions are tracked only when invoked via Claude Code hook)", target)
	}

	var totals *transcript.SessionTotals
	if transcriptPath != "" {
		totals, _ = transcript.Read(transcriptPath)
	}

	fmt.Print(analytics.RenderSessionGain(target, sessionRecords, totals))
	return nil
}
