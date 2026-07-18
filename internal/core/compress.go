package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode"

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
			if reversed, ok := applyReversible(r.Command, filtered, opts.StashMinBytes, opts.Prose); ok {
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
func applyReversible(command, content string, minBytes int, prose ProseFunc) (string, bool) {
	if minBytes <= 0 {
		minBytes = stash.MinSize()
	}
	if len(content) < minBytes {
		return "", false
	}
	id, err := stash.Put(command, content)
	if err != nil {
		return "", false
	}

	// Everything downstream — prose compression and the head/tail preview
	// fallback — works far better on visible text than on raw HTML: markup is
	// symbol-dense, so it never looks like prose (looksLikeProse rejects the
	// leading '<') and its head/tail is a wall of tag soup. Extract the text
	// once and drive both paths from it. The full original HTML is stashed
	// above (recoverable via td_retrieve), so this stays quality-neutral.
	base := content
	if filter.LooksLikeHTML(content) {
		base = filter.HTMLToText(content)
	}

	// Prose-aware preview: for natural-language content, a learned compressor
	// keeps high-signal tokens throughout instead of crude head/tail
	// truncation. It's lossy, but the full original is stashed above
	// (recoverable via td_retrieve), so it stays quality-neutral by the
	// recover-rate measure the eval harness checks. Only runs when a prose
	// compressor was injected AND the content actually looks like prose
	// (never on logs/JSON/code — see looksLikeProse).
	if prose != nil && looksLikeProse(base) {
		if compressed, ok := prose(base); ok {
			out := compressed + "\n" + stash.Marker(id, len(content))
			if len(out) < len(content) {
				return out, true
			}
		}
	}

	preview := stash.Preview(id, base, previewHeadLines, previewTailLines)
	// stash.Preview returns short content verbatim, embedding no marker. When
	// base is a lossy HTML reduction, that verbatim text must still carry a
	// retrieval marker or the dropped markup would be silently unrecoverable —
	// append one. (For non-HTML, base == content and the verbatim return is
	// itself lossless, so no marker is needed.)
	if base != content && preview == base {
		preview = base + "\n" + stash.Marker(id, len(content))
	}
	if len(preview) >= len(content) {
		return "", false
	}
	return preview, true
}

// looksLikeProse is a conservative heuristic: natural-language text is
// letter-heavy and flows in long lines/paragraphs, unlike JSON/XML (symbol
// prefixes), logs (many short, digit/punctuation-heavy lines), or code
// (symbol-dense). When in doubt it returns false, so the safe head/tail
// preview is used instead of an ML compressor that isn't meant for structure.
func looksLikeProse(s string) bool {
	t := strings.TrimSpace(s)
	if t == "" {
		return false
	}
	switch t[0] {
	case '{', '[', '<': // JSON / XML / HTML
		return false
	}
	letters, nonSpace := 0, 0
	for _, r := range t {
		if r == ' ' || r == '\n' || r == '\t' || r == '\r' {
			continue
		}
		nonSpace++
		if unicode.IsLetter(r) {
			letters++
		}
	}
	if nonSpace == 0 {
		return false
	}
	// Prose is letter-dominant; logs/code/tables carry many digits & symbols.
	if float64(letters)/float64(nonSpace) < 0.75 {
		return false
	}
	// Prose flows in long lines; logs/tables are many short ones.
	lines := strings.Count(t, "\n") + 1
	return len(t)/lines >= 40
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
