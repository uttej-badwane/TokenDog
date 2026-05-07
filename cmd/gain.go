package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"tokendog/internal/analytics"
	"tokendog/internal/transcript"
)

var (
	gainHistory   bool
	gainSession   string
	gainByModel   bool
	gainByProject bool
	gainDaily     bool
	gainMonthly   bool
	gainSince     string
	gainUntil     string
	gainJSON      bool
	gainWithSpend bool
)

var gainCmd = &cobra.Command{
	Use:   "gain",
	Short: "Show token savings summary",
	Long: `Aggregate TD's analytics history into a savings summary.

Flags compose: --by-model and --daily can both be set to produce a daily
breakdown with a per-model footer. --json emits machine-readable output
suitable for piping into dashboards or other tools (ccusage, jq, etc.).

Date filters accept ISO dates (2026-04-01) or relative durations (7d, 1m).`,
	RunE: runGain,
}

func init() {
	gainCmd.Flags().BoolVar(&gainHistory, "history", false, "Show recent command history")
	gainCmd.Flags().StringVar(&gainSession, "session", "", "Show savings for a specific session id (or 'current' for the active session)")
	gainCmd.Flags().BoolVar(&gainByModel, "by-model", false, "Show per-model breakdown using model-specific Anthropic rates")
	gainCmd.Flags().BoolVar(&gainByProject, "by-project", false, "Show per-project breakdown (resolved via .git root above the cwd at exec time)")
	gainCmd.Flags().BoolVar(&gainDaily, "daily", false, "Aggregate by calendar day")
	gainCmd.Flags().BoolVar(&gainMonthly, "monthly", false, "Aggregate by calendar month")
	gainCmd.Flags().StringVar(&gainSince, "since", "", "Only include records on or after this date (YYYY-MM-DD or relative like 7d/1m)")
	gainCmd.Flags().StringVar(&gainUntil, "until", "", "Only include records on or before this date (YYYY-MM-DD or relative like 7d/1m)")
	gainCmd.Flags().BoolVar(&gainJSON, "json", false, "Emit JSON instead of the human-readable table")
	gainCmd.Flags().BoolVar(&gainWithSpend, "with-spend", false, "Cross-reference ccusage spend data (requires npx ccusage on PATH)")
}

func runGain(_ *cobra.Command, _ []string) error {
	records, err := analytics.LoadAll()
	if err != nil {
		return err
	}

	if gainSince != "" || gainUntil != "" {
		since, until, err := parseDateRange(gainSince, gainUntil)
		if err != nil {
			return err
		}
		records = filterByDate(records, since, until)
	}

	// Resolve per-record Model from transcripts. Cheap (cached per session)
	// and necessary for both per-model breakdown and accurate cost.
	analytics.ResolveModels(records)

	if gainSession != "" {
		return runGainSession(records)
	}

	if gainJSON {
		return runGainJSON(records)
	}

	if gainDaily || gainMonthly {
		fmt.Print(analytics.RenderTimeSeries(records, gainMonthly, gainByModel))
		return nil
	}

	out := analytics.RenderGain(records, gainHistory)
	if gainByModel {
		out += analytics.RenderByModel(records)
	}
	if gainByProject {
		out += analytics.RenderByProject(records)
	}
	if gainWithSpend {
		out += renderWithSpend(records)
	}
	fmt.Print(out)
	return nil
}

// renderWithSpend invokes ccusage daily --json (npx-style if not directly
// on PATH) and joins its lifetime spend with td's lifetime savings.
// Format: "you spent $X on Anthropic; td saved $Y of it (Z%)".
//
// Quietly degrades: if ccusage isn't installed or errors, we print a
// one-line note and continue. td gain --with-spend should never make the
// rest of the command fail.
func renderWithSpend(records []analytics.Record) string {
	out, err := callCCUsage()
	if err != nil {
		return fmt.Sprintf("\n[--with-spend skipped: %s — install via `npm i -g ccusage`]\n", err.Error())
	}
	summary, _ := analytics.Summarize(records)
	tdUSD := summary.USDSaved()

	var b strings.Builder
	dash := strings.Repeat("─", 60)
	b.WriteString("\nYour Anthropic spend (via ccusage)\n")
	b.WriteString(dash + "\n")
	b.WriteString(fmt.Sprintf("  %-22s $%.2f\n", "Total spend:", out.totalUSD))
	b.WriteString(fmt.Sprintf("  %-22s $%.4f\n", "TD saved (lifetime):", tdUSD))
	if out.totalUSD > 0 {
		pct := tdUSD / out.totalUSD * 100
		b.WriteString(fmt.Sprintf("  %-22s %.2f%%\n", "TD share of bill:", pct))
	}
	b.WriteString(dash + "\n")
	return b.String()
}

type ccusageOut struct {
	totalUSD float64
}

// callCCUsage runs `ccusage daily --json` (preferring `ccusage` directly,
// falling back to `npx ccusage@latest`) and parses the totalCost. We
// don't try to model the full ccusage schema; we just want the headline
// total.
func callCCUsage() (ccusageOut, error) {
	commands := [][]string{
		{"ccusage", "daily", "--json"},
		{"npx", "-y", "ccusage@latest", "daily", "--json"},
	}
	var lastErr error
	for _, cmdParts := range commands {
		bin, args := cmdParts[0], cmdParts[1:]
		if _, err := exec.LookPath(bin); err != nil {
			lastErr = err
			continue
		}
		c := exec.Command(bin, args...)
		out, err := c.Output()
		if err != nil {
			lastErr = err
			continue
		}
		var parsed struct {
			Daily []struct {
				TotalCost float64 `json:"totalCost"`
			} `json:"daily"`
		}
		if err := json.Unmarshal(out, &parsed); err != nil {
			lastErr = err
			continue
		}
		total := 0.0
		for _, d := range parsed.Daily {
			total += d.TotalCost
		}
		return ccusageOut{totalUSD: total}, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("ccusage not found on PATH")
	}
	return ccusageOut{}, lastErr
}

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

	if gainJSON {
		return emitJSON(map[string]any{
			"session_id": target,
			"records":    sessionRecords,
			"transcript": totals,
		})
	}
	fmt.Print(analytics.RenderSessionGain(target, sessionRecords, totals))
	return nil
}

func runGainJSON(records []analytics.Record) error {
	summary, byCmd := analytics.Summarize(records)
	payload := map[string]any{
		"summary":      summary,
		"by_command":   byCmd,
		"record_count": len(records),
	}
	if gainDaily || gainMonthly {
		payload["time_series"] = analytics.TimeSeriesData(records, gainMonthly, gainByModel)
	}
	return emitJSON(payload)
}

func emitJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// parseDateRange resolves --since / --until strings to time.Time. Supports
// ISO (2026-04-01), compact (20260401), and relative (7d, 2w, 1m, 1y).
// An empty string means "no bound" — returned as zero time.
func parseDateRange(sinceStr, untilStr string) (time.Time, time.Time, error) {
	since, err := parseDateOrDuration(sinceStr)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("--since %q: %w", sinceStr, err)
	}
	until, err := parseDateOrDuration(untilStr)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("--until %q: %w", untilStr, err)
	}
	return since, until, nil
}

func parseDateOrDuration(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	// Relative: "7d", "2w", "1m", "1y"
	if len(s) > 1 {
		unit := s[len(s)-1]
		if n, err := strconv.Atoi(s[:len(s)-1]); err == nil {
			now := time.Now()
			switch unit {
			case 'd':
				return now.AddDate(0, 0, -n), nil
			case 'w':
				return now.AddDate(0, 0, -7*n), nil
			case 'm':
				return now.AddDate(0, -n, 0), nil
			case 'y':
				return now.AddDate(-n, 0, 0), nil
			}
		}
	}
	// Absolute: try ISO (2026-04-01), then compact (20260401).
	for _, layout := range []string{"2006-01-02", "20060102"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized date format (try YYYY-MM-DD or Nd/Nw/Nm/Ny)")
}

func filterByDate(records []analytics.Record, since, until time.Time) []analytics.Record {
	if since.IsZero() && until.IsZero() {
		return records
	}
	out := records[:0]
	for _, r := range records {
		if !since.IsZero() && r.Timestamp.Before(since) {
			continue
		}
		if !until.IsZero() && r.Timestamp.After(until) {
			continue
		}
		out = append(out, r)
	}
	return out
}
