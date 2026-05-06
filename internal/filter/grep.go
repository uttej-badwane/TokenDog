package filter

import (
	"strings"
	"unicode"
)

// Grep compresses `grep -r` style output by grouping matches under their
// file path. Standard grep emits one line per match in the form
// `path:lineno:content` — when many matches share the same file, the path
// dominates the byte count for no information gain. We collapse runs of
// matches with identical path to:
//
//	path/to/file:
//	  42: matched content
//	  56: another match
//
// Lossless: every matched line + line number + content is preserved. Order
// is preserved. Lines that don't fit the `path:lineno:content` shape
// (error messages, "Binary file foo matches" notices, non-grep stdout)
// pass through unchanged.
//
// The filter activates only when the input shows the multi-file pattern.
// Single-file output, filename-only output (-l), and count output (-c)
// pass through — there's nothing to dedupe.
func Grep(args []string, raw string) string {
	if raw == "" {
		return raw
	}
	lines := strings.Split(raw, "\n")
	// Trim a single trailing empty line so we don't emit a phantom
	// pseudo-match at EOF.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	var groups []grepGroup
	var passthrough []string // lines that don't fit the path:lineno:content shape
	matchedLines := 0
	for _, line := range lines {
		path, lineno, content, ok := parseGrepMatch(line)
		if !ok {
			passthrough = append(passthrough, line)
			continue
		}
		matchedLines++
		// Append to the current group if path matches the trailing one,
		// else start a new group. We only group consecutive same-path
		// matches — interleaved output (rare with -r but possible) keeps
		// its order.
		if n := len(groups); n > 0 && groups[n-1].path == path {
			groups[n-1].matches = append(groups[n-1].matches, grepMatch{lineno, content})
		} else {
			groups = append(groups, grepGroup{path: path, matches: []grepMatch{{lineno, content}}})
		}
	}

	// If no lines parsed as grep matches, or every group has a single
	// match, grouping wins us nothing (the `path:\n  ` overhead would
	// inflate). Pass through.
	if matchedLines == 0 || allSingleMatch(groups) {
		return raw
	}

	var b strings.Builder
	for _, g := range groups {
		// One-match groups stay in the original `path:lineno:content` form
		// — indenting them costs more than it saves.
		if len(g.matches) == 1 {
			m := g.matches[0]
			b.WriteString(g.path)
			b.WriteByte(':')
			b.WriteString(m.lineno)
			b.WriteByte(':')
			b.WriteString(m.content)
			b.WriteByte('\n')
			continue
		}
		b.WriteString(g.path)
		b.WriteString(":\n")
		for _, m := range g.matches {
			b.WriteString("  ")
			b.WriteString(m.lineno)
			b.WriteString(": ")
			b.WriteString(m.content)
			b.WriteByte('\n')
		}
	}
	for _, line := range passthrough {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// grepAdapter wires Grep into the filter registry.
func grepAdapter(args []string, raw string) string {
	return Grep(args, raw)
}

type grepGroup struct {
	path    string
	matches []grepMatch
}

type grepMatch struct {
	lineno  string
	content string
}

// parseGrepMatch extracts (path, lineno, content) from a `grep -n` style
// line. Returns ok=false when the line doesn't fit, signaling passthrough.
//
// Path detection: scan for the first colon followed by an unsigned
// integer followed by a colon. That digit run is the line number;
// everything to its left is the path; everything to the right is the
// content. Handles paths with embedded colons before the lineno
// (Windows `C:\path:42:line`) without a regex.
func parseGrepMatch(line string) (path, lineno, content string, ok bool) {
	if line == "" || line[0] == ' ' || line[0] == '\t' {
		return "", "", "", false
	}
	for i := 0; i < len(line); i++ {
		if line[i] != ':' {
			continue
		}
		j := i + 1
		for j < len(line) && unicode.IsDigit(rune(line[j])) {
			j++
		}
		if j == i+1 {
			continue
		}
		if j >= len(line) || line[j] != ':' {
			continue
		}
		if i == 0 {
			return "", "", "", false
		}
		return line[:i], line[i+1 : j], line[j+1:], true
	}
	return "", "", "", false
}

// allSingleMatch reports whether every group has exactly one match. When
// true the filter passes through — there's no compression opportunity.
func allSingleMatch(groups []grepGroup) bool {
	for _, g := range groups {
		if len(g.matches) > 1 {
			return false
		}
	}
	return true
}
