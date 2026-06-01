package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"tokendog/internal/filter"
	"tokendog/internal/hook"
	"tokendog/internal/stash"
)

// previewHeadLines / previewTailLines bound the head/tail kept when a large
// output is stashed for reversible compression.
const (
	previewHeadLines = 20
	previewTailLines = 5
)

// Compress runs the engine over a Conversation. For each eligible result it
// tries, in order: dedup → per-tool filter → generic compaction → reversible
// stash. The first that wins replaces the text. It mutates conv.Results in
// place (setting Replacement/Replaced) and returns the savings for the
// frontend to record. Pure: no HTTP, no analytics, no provider knowledge.
func Compress(conv *Conversation, opts Options) []Saving {
	if conv == nil || len(conv.Results) == 0 {
		return nil
	}

	// Index every NON-eligible (earlier-turn) result by content hash so an
	// eligible duplicate can be replaced with a back-reference. Keep the most
	// recent occurrence (smallest back-distance). Ordinals run across all
	// results in order so the marker can report distance.
	priors := map[string]priorRef{}
	for i, r := range conv.Results {
		if r.Eligible || r.Text == "" {
			continue
		}
		priors[hashText(r.Text)] = priorRef{cmd: r.Command, ordinal: i + 1}
	}

	var savings []Saving
	for i, r := range conv.Results {
		if !r.Eligible || r.Text == "" {
			continue
		}
		ordinal := i + 1
		raw := r.Text

		// Dedup — highest-value single substitution, command-agnostic.
		if opts.Dedup {
			if deduped, ok := applyDedup(raw, priors, ordinal); ok {
				r.Replacement, r.Replaced = deduped, true
				savings = append(savings, Saving{dedupLabel(r.Command), raw, deduped})
				continue
			}
		}

		// The remaining passes need a recognized shell command.
		if r.Command == "" {
			continue
		}

		// Per-tool lossless filter, then generic JSON fallback when no
		// per-tool filter claimed the output.
		filtered := raw
		applied := false
		if bin, args, ok := hook.ParseBinary(r.Command); ok {
			if f, a := filter.Apply(bin, args, raw); a {
				filtered, applied = f, a
			}
		}
		if !applied {
			if g, ok := filter.Generic(raw); ok {
				filtered, applied = g, true
			}
		}

		// Reversible stash of whatever's left (opt-in).
		if opts.Reversible {
			if reversed, ok := applyReversible(r.Command, filtered); ok {
				r.Replacement, r.Replaced = reversed, true
				savings = append(savings, Saving{r.Command + " (reversible)", raw, reversed})
				continue
			}
		}

		if !applied || filtered == raw {
			continue
		}
		r.Replacement, r.Replaced = filtered, true
		savings = append(savings, Saving{r.Command, raw, filtered})
	}
	return savings
}

// priorRef records where an earlier identical result appeared.
type priorRef struct {
	cmd     string
	ordinal int
}

func hashText(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:16]
}

// applyDedup returns a back-reference marker for raw if an identical output
// appeared earlier and the marker is strictly smaller (the Guard invariant).
func applyDedup(raw string, priors map[string]priorRef, currentOrdinal int) (string, bool) {
	prior, ok := priors[hashText(raw)]
	if !ok {
		return "", false
	}
	marker := dedupMarker(prior, currentOrdinal-prior.ordinal, len(raw))
	if len(marker) >= len(raw) {
		return "", false
	}
	return marker, true
}

func dedupMarker(prior priorRef, outputsBack, origBytes int) string {
	where := fmt.Sprintf("%d tool output(s) earlier", outputsBack)
	if outputsBack == 1 {
		where = "the previous tool output"
	}
	cmd := ""
	if prior.cmd != "" {
		cmd = " of `" + prior.cmd + "`"
	}
	return fmt.Sprintf(
		"[td: identical to the output%s — %s in this conversation. Elided "+
			"to save tokens; the full %s text appears verbatim above.]",
		cmd, where, humanBytes(origBytes))
}

// dedupLabel builds the analytics label for a deduped result.
func dedupLabel(cmd string) string {
	if cmd == "" {
		return "dedup"
	}
	return cmd + " (dedup)"
}

// applyReversible stashes content and returns a compact head/tail preview
// when the content is large enough to be worth it. Returns ("", false) when
// it's too small, eliding wouldn't shrink it, or the stash write fails — in
// every false case the caller keeps the lossless path.
func applyReversible(command, content string) (string, bool) {
	if len(content) < stash.MinSize() {
		return "", false
	}
	id, err := stash.Put(command, content)
	if err != nil {
		return "", false
	}
	preview := stash.Preview(id, content, previewHeadLines, previewTailLines)
	if len(preview) >= len(content) {
		return "", false
	}
	return preview, true
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
