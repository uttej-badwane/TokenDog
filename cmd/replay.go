package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"tokendog/internal/filter"
	"tokendog/internal/hook"
	"tokendog/internal/replay"
)

var (
	replayDays        int
	replayMaxSessions int
	replayTopN        int
	replayPriceM      float64
)

var replayCmd = &cobra.Command{
	Use:   "replay",
	Short: "Replay your historical Claude transcripts to project counterfactual savings",
	Long: `Walks every Claude Code transcript at ~/.claude/projects/, replays each
historical Bash tool_result through TokenDog's current filters, and reports
how many tokens (and dollars) you would have saved if td had been active.

Useful as a "what's the upside" estimate before installing td everywhere,
and as a calibration check on filter quality across real-world output.`,
	RunE: runReplay,
}

func init() {
	replayCmd.Flags().IntVar(&replayDays, "days", 0, "Only replay sessions modified within the last N days (0 = all time)")
	replayCmd.Flags().IntVar(&replayMaxSessions, "max-sessions", 0, "Cap on number of sessions to replay (newest first; 0 = no cap)")
	replayCmd.Flags().IntVar(&replayTopN, "top", 15, "How many top commands to show in the breakdown")
	replayCmd.Flags().Float64Var(&replayPriceM, "price-per-million", 15.0, "$ per million input tokens (Opus 4.7 standard = 15)")
}

func runReplay(_ *cobra.Command, _ []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	root := filepath.Join(home, ".claude", "projects")
	if _, err := os.Stat(root); err != nil {
		return fmt.Errorf("no Claude transcripts found at %s — is Claude Code installed?", root)
	}

	opts := replay.Options{MaxSessions: replayMaxSessions}
	if replayDays > 0 {
		opts.Since = time.Now().AddDate(0, 0, -replayDays)
	}

	fmt.Fprintf(os.Stderr, "Scanning transcripts...\n")
	r, err := replay.Walk(root, dispatchReplay, opts)
	if err != nil {
		return err
	}
	fmt.Print(renderReplay(r, replayTopN, replayPriceM))
	return nil
}

// dispatchReplay is the binary→filter routing replay calls back into. Mirrors
// what cmd/run.go's runFiltered does for live commands. Returns Handled=true
// whenever the binary is in hook.Supported, regardless of whether the filter
// produced a savings — a no-op filter on already-minimal output is still a
// "TD touched it" event for accounting purposes.
func dispatchReplay(command, raw string) (string, replay.DispatchInfo) {
	bin, args, ok := hook.ParseBinary(command)
	if !ok {
		return raw, replay.DispatchInfo{Handled: false}
	}
	filtered := filter.Guard(raw, applyFilter(bin, args, raw))
	return filtered, replay.DispatchInfo{
		Handled: true,
		Binary:  bin,
		Subcmd:  subcmdForBinary(bin, args),
	}
}

// subcmdForBinary picks the subcommand the live runner would have used for
// per-tool grouping. Mirrors the value-flag-aware extraction in cmd/run.go.
// For tools without a meaningful subcommand concept (find, ls, jq, curl,
// make, package managers, cloud CLIs), returns "" so the key is just the
// binary name.
func subcmdForBinary(bin string, args []string) string {
	switch bin {
	case "git":
		return extractSubcommand(args, gitValueFlags)
	case "gh":
		return extractSubcommand(args, ghValueFlags)
	case "docker":
		return extractSubcommand(args, dockerValueFlags)
	case "kubectl":
		return extractSubcommand(args, kubectlValueFlags)
	case "cargo":
		return extractSubcommand(args, cargoValueFlags)
	case "go":
		return firstNonFlag(args)
	}
	return ""
}

// applyFilter dispatches by binary name. Mirrors the registrations in
// cmd/root.go — every cobra subcommand gets a clause here so live and
// replay savings stay consistent. If you add a new filter live, add it
// here too or replay will under-report.
func applyFilter(binary string, args []string, raw string) string {
	switch binary {
	case "git":
		return filter.Git(extractSubcommand(args, gitValueFlags), raw)
	case "ls":
		return filter.Ls(raw)
	case "find":
		return filter.Find(raw)
	case "docker":
		return filter.Docker(extractSubcommand(args, dockerValueFlags), raw)
	case "jq":
		return filter.JQ(raw)
	case "curl":
		return filter.Curl(raw)
	case "kubectl":
		return filter.Kubectl(extractSubcommand(args, kubectlValueFlags), raw)
	case "gh":
		return filter.GH(extractSubcommand(args, ghValueFlags), raw)
	case "pytest":
		return filter.Test("pytest", raw)
	case "jest":
		return filter.Test("jest", raw)
	case "vitest":
		return filter.Test("vitest", raw)
	case "go":
		// Only `go test` is filtered live; mirror that.
		if firstNonFlag(args) == "test" {
			return filter.Test("go", raw)
		}
		return raw
	case "cargo":
		switch extractSubcommand(args, cargoValueFlags) {
		case "test":
			return filter.Test("cargo", raw)
		case "build", "check", "fetch", "update":
			return filter.PackageManager(raw)
		}
		return raw
	case "npm", "pnpm", "yarn", "pip":
		return filter.PackageManager(raw)
	case "aws", "gcloud", "az":
		return filter.Cloud(raw)
	case "make":
		return filter.Make(raw)
	}
	return raw
}

func firstNonFlag(args []string) string {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			return a
		}
	}
	return ""
}

// renderReplay produces the human-readable summary. Three sections: headline
// totals, per-command breakdown, top "would-have-helped-but-no-filter"
// binaries (so users can request new filters where they'd help most).
func renderReplay(r *replay.Result, topN int, pricePerM float64) string {
	var b strings.Builder
	sep := strings.Repeat("═", 64)
	dash := strings.Repeat("─", 72)

	b.WriteString("TokenDog Hindsight\n")
	b.WriteString(sep + "\n\n")

	if r.SessionsScanned == 0 {
		b.WriteString("No transcripts found.\n")
		return b.String()
	}

	b.WriteString(fmt.Sprintf("%-26s %d sessions, %d Bash calls (%d handled by td filters)\n",
		"Replayed:", r.SessionsScanned, r.BashCallsSeen, r.BashCallsHandled))
	b.WriteString(fmt.Sprintf("%-26s %s raw → %s filtered\n",
		"Output volume:", humanBytesReplay(r.RawBytes), humanBytesReplay(r.FilteredBytes)))
	b.WriteString(fmt.Sprintf("%-26s %d tokens (%.1f%% reduction, cl100k)\n",
		"Would-have-saved:", r.TokensSaved(), r.SavedPct()))

	usd := float64(r.TokensSaved()) / 1_000_000 * pricePerM
	b.WriteString(fmt.Sprintf("%-26s $%.2f at $%.0f/M input tokens\n", "Projected cost saved:", usd, pricePerM))
	b.WriteString("\n")

	// Per-command breakdown, sorted by tokens saved.
	cmds := make([]*replay.CommandStat, 0, len(r.PerCommand))
	for _, c := range r.PerCommand {
		cmds = append(cmds, c)
	}
	sort.Slice(cmds, func(i, j int) bool {
		return tokensSaved(cmds[i]) > tokensSaved(cmds[j])
	})

	b.WriteString("Top commands by projected savings\n")
	b.WriteString(dash + "\n")
	b.WriteString(fmt.Sprintf("  %-26s %-7s  %-12s  %-7s  %s\n",
		"Command", "Calls", "Tokens saved", "Pct", "Cost saved"))
	b.WriteString(dash + "\n")
	for i, c := range cmds {
		if i >= topN {
			break
		}
		ts := tokensSaved(c)
		pct := 0.0
		if c.RawTokens > 0 {
			pct = float64(ts) / float64(c.RawTokens) * 100
		}
		cmdUSD := float64(ts) / 1_000_000 * pricePerM
		name := c.Name
		if len(name) > 26 {
			name = name[:23] + "..."
		}
		b.WriteString(fmt.Sprintf("  %-26s %-7d  %-12d  %5.1f%%   $%.2f\n",
			name, c.Calls, ts, pct, cmdUSD))
	}
	b.WriteString(dash + "\n\n")

	// Sessions with biggest single-session savings — useful for "look how
	// much that one debugging session would have saved you" framing.
	sessions := make([]*replay.SessionStat, len(r.PerSession))
	copy(sessions, r.PerSession)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].TokensSaved > sessions[j].TokensSaved
	})

	b.WriteString("Top sessions by projected savings\n")
	b.WriteString(dash + "\n")
	b.WriteString(fmt.Sprintf("  %-12s  %-12s  %-7s  %-12s  %s\n",
		"Date", "Session", "Calls", "Tokens saved", "Cost saved"))
	b.WriteString(dash + "\n")
	for i, s := range sessions {
		if i >= 5 || s.TokensSaved == 0 {
			break
		}
		date := s.LastActivity.Format("2006-01-02")
		shortID := s.SessionID
		if len(shortID) > 12 {
			shortID = shortID[:8] + "…"
		}
		usd := float64(s.TokensSaved) / 1_000_000 * pricePerM
		b.WriteString(fmt.Sprintf("  %-12s  %-12s  %-7d  %-12d  $%.2f\n",
			date, shortID, s.BashCalls, s.TokensSaved, usd))
	}
	b.WriteString(dash + "\n\n")

	// "Add a filter for these" report — top unhandled binaries by call count.
	if len(r.UnhandledTopN) > 0 {
		type um struct {
			name  string
			count int
		}
		var unhandled []um
		for k, v := range r.UnhandledTopN {
			unhandled = append(unhandled, um{name: k, count: v})
		}
		sort.Slice(unhandled, func(i, j int) bool {
			return unhandled[i].count > unhandled[j].count
		})

		b.WriteString("Top unhandled commands (no td filter exists yet)\n")
		b.WriteString(dash + "\n")
		shown := 0
		for _, u := range unhandled {
			if u.count < 3 {
				continue
			}
			if shown >= 10 {
				break
			}
			b.WriteString(fmt.Sprintf("  %-26s %d calls\n", u.name, u.count))
			shown++
		}
		b.WriteString(dash + "\n\n")
	}

	b.WriteString("Caveats:\n")
	b.WriteString("  • Filter quality is today's; old outputs replayed through current code.\n")
	b.WriteString("  • tool_result content is what Claude saw — already capped by Claude Code's\n")
	b.WriteString("    own truncation. Real raw output may have been larger.\n")
	b.WriteString("  • cl100k tokens are an Anthropic proxy (~10% margin).\n")
	return b.String()
}

func tokensSaved(c *replay.CommandStat) int { return c.RawTokens - c.FilteredTokens }

func humanBytesReplay(n int) string {
	switch {
	case n >= 1024*1024*1024:
		return fmt.Sprintf("%.2fGB", float64(n)/1024/1024/1024)
	case n >= 1024*1024:
		return fmt.Sprintf("%.1fMB", float64(n)/1024/1024)
	case n >= 1024:
		return fmt.Sprintf("%.1fKB", float64(n)/1024)
	default:
		return fmt.Sprintf("%dB", n)
	}
}
