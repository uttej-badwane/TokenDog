// Package eval is TokenDog's offline quality harness. It answers the question
// a serious buyer asks before trusting any compression: "does this ever drop
// something the task needed?" — with numbers, deterministically, and for free.
//
// The design avoids the trap of needing a live model in CI. Each fixture
// declares the answer-bearing facts a downstream task would depend on
// (must_keep). The harness compresses the fixture through the real engine and
// checks, for every fact, whether it is still reachable. Two distinct
// measures, because they mean different things:
//
//   - inline:      the fact survives directly in the compressed text. No
//     retrieval needed — pure win.
//   - recoverable: the fact is reachable at all — inline, OR verbatim earlier
//     in the conversation (dedup back-reference), OR in the
//     reversible stash (the model can pull it back).
//
// A harness PASSES iff every fact is recoverable. That is the hard
// correctness gate: compression may defer a fact to a retrieval, but it must
// never destroy one. The inline rate is reported separately as an efficiency
// signal (lower inline = more retrieval round-trips), not a correctness gate.
//
// For lossless transforms (per-tool filter, generic JSON) inline == recoverable
// by construction — so this same harness is a regression gate proving a filter
// never strips an answer-bearing fact.
package eval

import (
	"regexp"
	"strings"

	"tokendog/internal/core"
	"tokendog/internal/stash"
)

// Fixture is one corpus entry: a tool output plus the facts a task would need
// from it. Prior, when set, is an identical earlier output used to exercise
// cross-message dedup (it is placed in the conversation as a non-eligible
// result so the eligible copy can back-reference it).
type Fixture struct {
	Name     string   `json:"name"`
	Command  string   `json:"command"`
	Output   string   `json:"output"`
	Prior    string   `json:"prior,omitempty"`
	MustKeep []string `json:"must_keep"`
	Options  struct {
		Dedup      bool `json:"dedup"`
		Reversible bool `json:"reversible"`
	} `json:"options"`
}

// Result is one fixture's outcome.
type Result struct {
	Name        string  `json:"name"`
	Transform   string  `json:"transform"` // lossless | dedup | reversible | none
	RawBytes    int     `json:"raw_bytes"`
	CompBytes   int     `json:"comp_bytes"`
	Ratio       float64 `json:"ratio"` // CompBytes/RawBytes (lower = more compression)
	Facts       int     `json:"facts"`
	Inline      int     `json:"inline"`
	Recoverable int     `json:"recoverable"`
}

// Lost reports facts that were neither inline nor recoverable — a correctness
// failure.
func (r Result) Lost() int { return r.Facts - r.Recoverable }

// Report aggregates a run.
type Report struct {
	Results      []Result `json:"results"`
	TotalRaw     int      `json:"total_raw"`
	TotalComp    int      `json:"total_comp"`
	TotalFacts   int      `json:"total_facts"`
	TotalInline  int      `json:"total_inline"`
	TotalRecover int      `json:"total_recoverable"`
	OverallRatio float64  `json:"overall_ratio"`
	InlineRate   float64  `json:"inline_rate"`
	RecoverRate  float64  `json:"recover_rate"`
	Pass         bool     `json:"pass"`
}

var stashMarkerRE = regexp.MustCompile(`\[td:STASHED id=([0-9a-f]+)`)

// Run compresses every fixture through the engine and scores fact survival.
// Deterministic and side-effect-light: reversible fixtures write to the stash
// (so the harness can verify recoverability by reading it back), which is the
// same store the production path uses.
func Run(fixtures []Fixture) Report {
	var rep Report
	for _, f := range fixtures {
		rep.Results = append(rep.Results, runOne(f))
	}
	for _, r := range rep.Results {
		rep.TotalRaw += r.RawBytes
		rep.TotalComp += r.CompBytes
		rep.TotalFacts += r.Facts
		rep.TotalInline += r.Inline
		rep.TotalRecover += r.Recoverable
	}
	if rep.TotalRaw > 0 {
		rep.OverallRatio = float64(rep.TotalComp) / float64(rep.TotalRaw)
	}
	if rep.TotalFacts > 0 {
		rep.InlineRate = float64(rep.TotalInline) / float64(rep.TotalFacts)
		rep.RecoverRate = float64(rep.TotalRecover) / float64(rep.TotalFacts)
	}
	rep.Pass = rep.TotalRecover == rep.TotalFacts
	return rep
}

func runOne(f Fixture) Result {
	conv := &core.Conversation{}
	if f.Prior != "" {
		conv.Results = append(conv.Results, &core.ToolResult{
			Command: f.Command, Text: f.Prior, Eligible: false,
		})
	}
	elig := &core.ToolResult{Command: f.Command, Text: f.Output, Eligible: true}
	conv.Results = append(conv.Results, elig)

	core.Compress(conv, core.Options{Dedup: f.Options.Dedup, Reversible: f.Options.Reversible})

	compText := elig.Text
	transform := "none"
	if elig.Replaced {
		compText = elig.Replacement
		transform = classify(compText)
	}

	// inline   = the literal bytes the model receives (stash markers NOT
	//            expanded) — facts reachable with no retrieval call. A dedup
	//            back-reference's original is inline because it sits verbatim
	//            earlier in the same prompt.
	// recover  = inline PLUS anything a td_retrieve call could pull from the
	//            stash. The correctness gate.
	inlineCtx := contextString(conv, false)
	recoverCtx := contextString(conv, true)

	res := Result{
		Name:      f.Name,
		Transform: transform,
		RawBytes:  len(f.Output),
		CompBytes: len(compText),
		Facts:     len(f.MustKeep),
	}
	if res.RawBytes > 0 {
		res.Ratio = float64(res.CompBytes) / float64(res.RawBytes)
	}
	for _, k := range f.MustKeep {
		if strings.Contains(inlineCtx, k) {
			res.Inline++
		}
		if strings.Contains(recoverCtx, k) {
			res.Recoverable++
		}
	}
	return res
}

// contextString concatenates every result's effective text. When expand is
// true, stash markers are replaced by the recovered original (modelling a
// td_retrieve call); when false, the literal prompt bytes are returned.
func contextString(conv *core.Conversation, expand bool) string {
	var b strings.Builder
	for _, r := range conv.Results {
		text := r.Text
		if r.Replaced {
			text = r.Replacement
			if expand {
				if m := stashMarkerRE.FindStringSubmatch(r.Replacement); m != nil {
					if rec, ok := stash.Get(m[1]); ok {
						text = rec.Content
					}
				}
			}
		}
		b.WriteString(text)
		b.WriteByte('\n')
	}
	return b.String()
}

func classify(replacement string) string {
	switch {
	// The reversible preview carries the [td:STASHED …] marker between its
	// head and tail lines, so match anywhere, not just the prefix.
	case strings.Contains(replacement, "[td:STASHED"):
		return "reversible"
	case strings.HasPrefix(replacement, "[td: identical"):
		return "dedup"
	default:
		return "lossless"
	}
}
