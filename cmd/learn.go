package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"tokendog/internal/analytics"
	"tokendog/internal/stash"
)

var (
	learnJSON bool
	learnTop  int
)

var learnCmd = &cobra.Command{
	Use:   "learn",
	Short: "Mine reversible-compression telemetry to find over-aggressive previews",
	Long: `Closes the loop on reversible compression (TD_REVERSIBLE=1).

Every time the proxy stashes a large output it injects a head/tail preview;
every time the model pulls the full original back via the td_retrieve MCP
tool, that retrieval is logged. A high retrieve rate for a command means its
preview is dropping content the model needs — the elision is too aggressive
for that command's output shape.

  td learn            # per-command stashed vs retrieved, with suggestions
  td learn --top 5    # only the worst offenders
  td learn --json     # machine-readable

Stashed counts come from analytics (proxy "(reversible)" records); retrieval
counts come from ~/.config/tokendog/retrievals.jsonl.`,
	RunE: runLearn,
}

func init() {
	learnCmd.Flags().BoolVar(&learnJSON, "json", false, "Emit machine-readable JSON")
	learnCmd.Flags().IntVar(&learnTop, "top", 0, "Show only the top N commands by retrieve rate (0 = all)")
}

// learnRow is one command's stash-vs-retrieve picture.
type learnRow struct {
	Command   string  `json:"command"`
	Stashed   int     `json:"stashed"`
	Retrieved int     `json:"retrieved"`
	Rate      float64 `json:"rate"` // Retrieved/Stashed, 0..1
}

const learnFlagRate = 0.5 // at/above this retrieve rate, flag the command

func runLearn(_ *cobra.Command, _ []string) error {
	records, err := analytics.LoadAll()
	if err != nil {
		return err
	}
	retrievals, err := stash.LoadRetrievals()
	if err != nil {
		return err
	}

	stashed := map[string]int{}
	totalStashed := 0
	for _, r := range records {
		cmd, ok := reversibleCommand(r.Command)
		if !ok {
			continue
		}
		stashed[learnBinary(cmd)]++
		totalStashed++
	}

	retrieved := map[string]int{}
	for _, rv := range retrievals {
		retrieved[learnBinary(rv.Command)]++
	}

	// Union of binaries seen on either side.
	seen := map[string]bool{}
	for b := range stashed {
		seen[b] = true
	}
	for b := range retrieved {
		seen[b] = true
	}

	rows := make([]learnRow, 0, len(seen))
	for b := range seen {
		s, rt := stashed[b], retrieved[b]
		rate := 0.0
		if s > 0 {
			rate = float64(rt) / float64(s)
		}
		rows = append(rows, learnRow{Command: b, Stashed: s, Retrieved: rt, Rate: rate})
	}
	// Sort by rate desc, then retrieved desc, then name for stability.
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Rate != rows[j].Rate {
			return rows[i].Rate > rows[j].Rate
		}
		if rows[i].Retrieved != rows[j].Retrieved {
			return rows[i].Retrieved > rows[j].Retrieved
		}
		return rows[i].Command < rows[j].Command
	})
	if learnTop > 0 && len(rows) > learnTop {
		rows = rows[:learnTop]
	}

	if learnJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"total_stashed":   totalStashed,
			"total_retrieved": len(retrievals),
			"commands":        rows,
		})
	}
	renderLearn(rows, totalStashed, len(retrievals))
	return nil
}

func renderLearn(rows []learnRow, totalStashed, totalRetrieved int) {
	fmt.Println("TokenDog Learn — reversible-compression telemetry")
	fmt.Println(strings.Repeat("═", 58))

	if totalStashed == 0 {
		fmt.Println("No reversible-compression activity recorded yet.")
		fmt.Println("Enable it with TD_REVERSIBLE=1 so large outputs get stashed,")
		fmt.Println("then re-run `td learn` after some sessions.")
		return
	}

	fmt.Printf("Stashed (reversible) events:  %d\n", totalStashed)
	fmt.Printf("Retrievals logged:            %d\n\n", totalRetrieved)

	if totalRetrieved == 0 {
		fmt.Println("Every stashed preview served without the model needing the full")
		fmt.Println("original back. Reversible compression is a clean win on your")
		fmt.Println("workflow so far — no preview is too aggressive.")
		return
	}

	fmt.Println("Per-command retrieve rate (higher = previews too aggressive):")
	fmt.Printf("%-16s %8s %10s %7s\n", "COMMAND", "STASHED", "RETRIEVED", "RATE")
	var flagged []learnRow
	for _, r := range rows {
		marker := ""
		if r.Stashed > 0 && r.Rate >= learnFlagRate {
			marker = "  ← previews likely too aggressive"
			flagged = append(flagged, r)
		}
		fmt.Printf("%-16s %8d %10d %6.0f%%%s\n", r.Command, r.Stashed, r.Retrieved, r.Rate*100, marker)
	}

	if len(flagged) == 0 {
		fmt.Println("\nNo command crosses the 50% retrieve-rate line — previews are")
		fmt.Println("holding up well.")
		return
	}

	fmt.Println("\nSuggestions:")
	for _, r := range flagged {
		fmt.Printf("- %s: %.0f%% of stashed outputs were pulled back in full. The "+
			"head/tail\n  preview is dropping content the model needs. Consider raising "+
			"TD_STASH_MIN\n  so %s output isn't stashed, or treat it as a poor stash "+
			"candidate.\n", r.Command, r.Rate*100, r.Command)
	}
}

// reversibleCommand extracts the underlying command from a proxy "(reversible)"
// analytics record. Returns ("", false) for any other record. The stored shape
// is `proxy: <command> (reversible)`.
func reversibleCommand(recordCmd string) (string, bool) {
	s := strings.TrimPrefix(recordCmd, "proxy: ")
	if s == recordCmd {
		return "", false // no proxy prefix — not a proxy record
	}
	const suffix = " (reversible)"
	if !strings.HasSuffix(s, suffix) {
		return "", false
	}
	return strings.TrimSuffix(s, suffix), true
}

// learnBinary reduces a full command string to the binary token used for
// grouping: skips leading VAR=val env assignments, strips any directory
// prefix, and lowercases nothing (binaries are case-sensitive). Falls back to
// the whole string when it can't find a token.
func learnBinary(command string) string {
	fields := strings.Fields(command)
	for _, f := range fields {
		if strings.Contains(f, "=") && !strings.HasPrefix(f, "-") {
			// Looks like an env assignment (FOO=bar) — skip.
			if eq := strings.IndexByte(f, '='); eq > 0 && isEnvName(f[:eq]) {
				continue
			}
		}
		if i := strings.LastIndexByte(f, '/'); i >= 0 {
			f = f[i+1:]
		}
		if f != "" {
			return f
		}
	}
	if command == "" {
		return "(unknown)"
	}
	return command
}

// isEnvName reports whether s is a plausible environment-variable name
// (letters, digits, underscore; not starting with a digit).
func isEnvName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_':
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}
