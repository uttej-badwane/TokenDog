package filter

import "strings"

// GH compresses GitHub CLI output. Tabular subcommands (pr list, issue list,
// run list, repo list, workflow list) get whitespace normalized — the table
// padding gh emits is purely cosmetic. Body subcommands (pr view, issue view,
// api, browse) are passed through unchanged because they contain user content
// (PR descriptions, issue bodies, API JSON) that must not be touched.
//
// Log subcommands (`gh run view --log`) get a separate branch that strips
// the per-line `job\tstep\ttimestamp` prefix repetition — the dominant
// noise in CI logs.
func GH(subcommand string, output string) string {
	// First, try the run-log detection — works regardless of subcommand
	// because the output format itself is the signal. `gh run view --log`
	// has a tab-separated `job\tstep\ttimestamp content` shape that no
	// other gh command emits.
	if looksLikeRunLog(output) {
		return ghRunLog(output)
	}
	switch subcommand {
	case "pr", "issue", "run", "repo", "workflow", "release", "cache", "ruleset", "label", "gist", "secret", "variable":
		return ghTable(output)
	case "api":
		// API responses are JSON or raw — try JSON compaction, fall back.
		return Curl(output)
	default:
		return output
	}
}

// looksLikeRunLog returns true when the output appears to be `gh run view
// --log` format: lines of the form `job\tstep\tISO-timestamp content`.
// Cheap detection: check the first non-empty line. BOM may appear before
// the timestamp on the very first line \u2014 gh's runner emits one \u2014 so we
// strip from each cell, not just the line.
func looksLikeRunLog(output string) bool {
	for _, line := range strings.SplitN(output, "\n", 4) {
		if line == "" {
			continue
		}
		line = strings.TrimPrefix(line, "\uFEFF")
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			return false
		}
		// Third column starts with a timestamp like "2026-05-06T20:03:10.8128553Z ".
		// Strip BOM here too \u2014 gh embeds it inside the third column on the
		// first line of multi-job runs.
		return looksLikeISOTimestamp(strings.TrimPrefix(parts[2], "\uFEFF"))
	}
	return false
}

// looksLikeISOTimestamp checks whether s starts with "YYYY-MM-DDTHH:MM:SS"
// followed by space (or fractional seconds). Avoids a regex on the hot
// path; format is fixed in GitHub Actions runners.
func looksLikeISOTimestamp(s string) bool {
	// Minimum: "2024-01-01T00:00:00Z " is 21 chars.
	if len(s) < 20 {
		return false
	}
	// Positions of expected separators in YYYY-MM-DDTHH:MM:SS.
	if s[4] != '-' || s[7] != '-' || s[10] != 'T' || s[13] != ':' || s[16] != ':' {
		return false
	}
	// All other positions in the date must be digits.
	for _, i := range []int{0, 1, 2, 3, 5, 6, 8, 9, 11, 12, 14, 15, 17, 18} {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// ghRunLog strips the per-line `job\tstep\ttimestamp` prefix from CI log
// output. Groups consecutive lines with the same job+step under a single
// header, replaces the wall-clock timestamp on each content line with a
// delta from the first timestamp in that group, and keeps the actual log
// content verbatim.
//
// Lossless contract:
//   - Every job, step, and content line is preserved
//   - First timestamp per group is emitted at the header (so absolute time
//     is recoverable)
//   - Subsequent timestamps become deltas (e.g. "+0.123s") — relative
//     ordering and durations are preserved
//
// The Guard at the wrapper-layer ensures we never inflate; if the input
// doesn't have the expected shape, the regex-fail path drops us back to
// passthrough naturally.
func ghRunLog(output string) string {
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if len(lines) == 0 {
		return output
	}

	type entry struct {
		job, step, ts, content string
	}
	parsed := make([]entry, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimPrefix(line, "\uFEFF")
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) == 3 {
			parts[2] = strings.TrimPrefix(parts[2], "\uFEFF")
		}
		if len(parts) != 3 || !looksLikeISOTimestamp(parts[2]) {
			// Pass-through line (unexpected format) — keep verbatim with
			// empty job/step so emit logic groups it into the prior group.
			parsed = append(parsed, entry{content: line})
			continue
		}
		// Third column is `<timestamp> <content>`. Find the first space
		// after the timestamp to split.
		col := parts[2]
		spaceIdx := strings.IndexByte(col, ' ')
		if spaceIdx < 0 {
			parsed = append(parsed, entry{job: parts[0], step: parts[1], ts: col})
			continue
		}
		parsed = append(parsed, entry{
			job:     parts[0],
			step:    parts[1],
			ts:      col[:spaceIdx],
			content: col[spaceIdx+1:],
		})
	}

	var b strings.Builder
	prevJob, prevStep := "", ""
	for _, e := range parsed {
		if e.job == "" && e.step == "" && e.ts == "" {
			// Unstructured passthrough line.
			b.WriteString(e.content)
			b.WriteByte('\n')
			continue
		}
		if e.job != prevJob || e.step != prevStep {
			// New section. Emit header with the absolute start timestamp.
			b.WriteString("=== ")
			b.WriteString(e.job)
			b.WriteString(" / ")
			b.WriteString(e.step)
			if e.ts != "" {
				b.WriteString(" @ ")
				b.WriteString(e.ts)
			}
			b.WriteString(" ===\n")
			prevJob, prevStep = e.job, e.step
		}
		b.WriteString(e.content)
		b.WriteByte('\n')
	}
	return b.String()
}

// ghTable normalizes column padding without losing data. gh's tables use
// runs of spaces (2+) to align columns; we collapse those to a single tab
// equivalent (two spaces) so the model still sees columnar structure.
//
// Lines that look like prose (no multi-space gaps, or starting with quoted
// text) are preserved verbatim — this protects PR/issue body content that
// `gh ... view` mixes in with metadata.
func ghTable(output string) string {
	if output == "" {
		return output
	}
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	var sb strings.Builder
	for _, line := range lines {
		if !looksTabular(line) {
			sb.WriteString(line)
			sb.WriteString("\n")
			continue
		}
		fields := splitOnMultiSpace(line)
		sb.WriteString(strings.Join(fields, "  "))
		sb.WriteString("\n")
	}
	filtered := sb.String()
	if len(filtered) >= len(output) {
		return output
	}
	return filtered
}

// looksTabular returns true when a line contains at least one run of 2+
// spaces between non-space tokens — gh's column separator.
func looksTabular(line string) bool {
	if strings.TrimSpace(line) == "" {
		return false
	}
	runStart := -1
	for i, r := range line {
		if r == ' ' {
			if runStart < 0 {
				runStart = i
			}
			if i-runStart >= 1 && i+1 < len(line) && line[i+1] != ' ' && runStart > 0 {
				return true
			}
		} else {
			runStart = -1
		}
	}
	return false
}

// splitOnMultiSpace splits a line on runs of 2+ spaces. Single spaces inside
// a column value (e.g. a PR title with words) are preserved.
func splitOnMultiSpace(line string) []string {
	var out []string
	var cur strings.Builder
	spaces := 0
	for _, r := range line {
		if r == ' ' {
			spaces++
			continue
		}
		if spaces >= 2 && cur.Len() > 0 {
			out = append(out, strings.TrimRight(cur.String(), " "))
			cur.Reset()
		} else if spaces == 1 && cur.Len() > 0 {
			cur.WriteByte(' ')
		}
		cur.WriteRune(r)
		spaces = 0
	}
	if cur.Len() > 0 {
		out = append(out, strings.TrimRight(cur.String(), " "))
	}
	return out
}
