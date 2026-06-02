package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"tokendog/internal/eval"
)

var (
	evalCorpus string
	evalJSON   bool
)

var evalCmd = &cobra.Command{
	Use:   "eval",
	Short: "Prove compression is quality-neutral on a corpus (numbers, not vibes)",
	Long: `Runs every fixture in a corpus through the real compression engine and
checks, for each answer-bearing fact the fixture declares (must_keep), whether
that fact survives.

Two measures are reported per fixture:

  inline       fact reachable in the prompt the model receives — no retrieval.
  recoverable  fact reachable at all — inline, OR via the reversible stash
               (a td_retrieve call), OR verbatim earlier in the conversation
               (a dedup back-reference).

The harness PASSES only if every fact is recoverable: compression may defer a
fact to a retrieval, but must never destroy one. The inline rate is reported
as an efficiency signal (lower = more retrieval round-trips), not a gate.

  td eval                       # run the built-in corpus
  td eval --corpus ./fixtures   # run your own *.json fixtures
  td eval --json                # machine-readable

Exits non-zero if any fact is lost, so it works as a CI gate.`,
	RunE:          runEval,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	evalCmd.Flags().StringVar(&evalCorpus, "corpus", "", "Directory of *.json fixtures (default: built-in corpus)")
	evalCmd.Flags().BoolVar(&evalJSON, "json", false, "Emit machine-readable JSON")
}

func runEval(_ *cobra.Command, _ []string) error {
	var (
		fixtures []eval.Fixture
		err      error
	)
	if evalCorpus != "" {
		fixtures, err = eval.LoadDir(evalCorpus)
	} else {
		fixtures, err = eval.LoadDefault()
	}
	if err != nil {
		return err
	}

	rep := eval.Run(fixtures)

	if evalJSON {
		return json.NewEncoder(os.Stdout).Encode(rep)
	}
	renderEval(rep)
	if !rep.Pass {
		return fmt.Errorf("eval FAILED: %d answer-bearing fact(s) lost", rep.TotalFacts-rep.TotalRecover)
	}
	return nil
}

func renderEval(rep eval.Report) {
	fmt.Printf("TokenDog Eval — %d fixtures\n", len(rep.Results))
	fmt.Println(strings.Repeat("═", 70))
	fmt.Printf("%-26s %-11s %6s %8s %9s\n", "FIXTURE", "TRANSFORM", "COMP%", "INLINE", "RECOVER")
	for _, r := range rep.Results {
		flag := ""
		if r.Lost() > 0 {
			flag = "  ← FACT LOST"
		} else if r.Recoverable > r.Inline {
			flag = fmt.Sprintf("  ← %d need retrieval", r.Recoverable-r.Inline)
		}
		fmt.Printf("%-26s %-11s %5.0f%% %7s %8s%s\n",
			truncate(r.Name, 26), r.Transform, r.Ratio*100,
			fmt.Sprintf("%d/%d", r.Inline, r.Facts),
			fmt.Sprintf("%d/%d", r.Recoverable, r.Facts), flag)
	}
	fmt.Println(strings.Repeat("─", 70))
	fmt.Printf("Aggregate: %s → %s (%.0f%% of original) · facts %d/%d recoverable (%.0f%%), %d/%d inline (%.0f%%)\n",
		humanBytes(rep.TotalRaw), humanBytes(rep.TotalComp), rep.OverallRatio*100,
		rep.TotalRecover, rep.TotalFacts, rep.RecoverRate*100,
		rep.TotalInline, rep.TotalFacts, rep.InlineRate*100)
	if rep.Pass {
		fmt.Println("RESULT: PASS — no answer-bearing fact lost")
	} else {
		fmt.Printf("RESULT: FAIL — %d fact(s) destroyed by compression\n", rep.TotalFacts-rep.TotalRecover)
	}
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n-1] + "…"
	}
	return s
}

func humanBytes(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1fKB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}
