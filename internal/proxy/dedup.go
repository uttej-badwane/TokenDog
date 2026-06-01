package proxy

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

// Cross-message dedup. Coding agents routinely re-emit identical tool output
// within a single conversation: re-reading the same file to re-check it,
// re-running a verbose status command, pasting the same config twice. Each
// repeat re-bills the full output even though a byte-identical copy already
// sits earlier in the prompt.
//
// When the LAST message's tool_result is byte-for-byte identical to a
// tool_result from an earlier message in the same request, we replace it with
// a short back-reference. This is:
//
//   - lossless — the full text is verbatim above, in the model's own context,
//     so nothing is actually removed from the conversation; and
//   - cache-safe — like every other proxy transform, it touches only the last
//     message, which Anthropic's prompt cache has not yet hashed.
//
// It deliberately works for ANY tool_result, not just Bash commands we have a
// filter for — re-reading a large file via the Read tool is one of the most
// common redundancies, and it has no per-tool filter at all.

const envNoDedup = "TD_NO_DEDUP"

// dedupDisabled reports whether cross-message dedup is turned off. It is on by
// default (lossless + cache-safe), with an escape hatch mirroring the command
// cache's TD_NO_CACHE.
func dedupDisabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(envNoDedup))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// priorRef records where an earlier identical tool_result appeared so the
// back-reference marker can point the model at it.
type priorRef struct {
	cmd     string // command that produced it, "" for non-Bash tools
	ordinal int    // 1-based position in tool-result order across the request
}

// hashText returns a short hex digest used as the dedup key. Short is fine:
// the set is one conversation's worth of tool_results, and a false positive
// would require a full sha256 collision.
func hashText(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:16]
}

// buildPriorResults indexes every tool_result in the given (earlier) messages
// by content hash, keeping the MOST RECENT occurrence so the back-reference
// distance is as small as possible. Returns the index plus the running
// tool-result count, which the caller continues from when numbering the last
// message's blocks.
func buildPriorResults(msgs []messageEntry, useByID map[string]string) (map[string]priorRef, int) {
	priors := map[string]priorRef{}
	ordinal := 0
	for _, m := range msgs {
		blocks, ok := unmarshalContent(m.Content)
		if !ok {
			continue
		}
		for _, b := range blocks {
			if b.Type != "tool_result" || b.ToolUseID == "" {
				continue
			}
			text := extractText(b.Content)
			if text == "" {
				continue
			}
			ordinal++
			priors[hashText(text)] = priorRef{cmd: useByID[b.ToolUseID], ordinal: ordinal}
		}
	}
	return priors, ordinal
}

// applyDedup returns a back-reference marker for raw if an identical output
// appeared earlier (and the marker is actually smaller — the Guard
// invariant). currentOrdinal is raw's own 1-based position so the marker can
// report how many tool outputs back the original is.
func applyDedup(raw string, priors map[string]priorRef, currentOrdinal int) (string, bool) {
	if dedupDisabled() {
		return "", false
	}
	prior, ok := priors[hashText(raw)]
	if !ok {
		return "", false
	}
	marker := dedupMarker(prior, currentOrdinal-prior.ordinal, len(raw))
	if len(marker) >= len(raw) {
		// Marker would cost more than it saves (tiny duplicated output).
		return "", false
	}
	return marker, true
}

// dedupMarker is the single line the model reads in place of the duplicate.
// It names the producing command when known and how far back the verbatim
// original is, so the model can locate it without a tool call.
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

// dedupLabel builds the analytics command label for a deduped result so
// `td gain` attributes the saving. Non-Bash tool_results have no command, so
// they fold into a single "dedup" bucket.
func dedupLabel(cmd string) string {
	if cmd == "" {
		return "dedup"
	}
	return cmd + " (dedup)"
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
