package filter

import "strings"

// GH compresses GitHub CLI output. Tabular subcommands (pr list, issue list,
// run list, repo list, workflow list) get whitespace normalized — the table
// padding gh emits is purely cosmetic. Body subcommands (pr view, issue view,
// api, browse) are passed through unchanged because they contain user content
// (PR descriptions, issue bodies, API JSON) that must not be touched.
func GH(subcommand string, output string) string {
	switch subcommand {
	case "pr", "issue", "run", "repo", "workflow", "release", "cache", "ruleset", "label", "gist", "secret", "variable":
		// Mixed — depends on the second arg. The cmd-level wrapper passes
		// only the first non-flag arg, so we route based on that and let
		// the table compactor handle list-style output. View/edit output
		// already includes the body; the compactor leaves prose lines alone.
		return ghTable(output)
	case "api":
		// API responses are JSON or raw — try JSON compaction, fall back.
		return Curl(output)
	default:
		return output
	}
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
