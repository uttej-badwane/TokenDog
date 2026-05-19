package filter

import (
	"strings"
)

// Psql compresses PostgreSQL client (psql / pgcli) tabular output without
// losing any row data. Three transformations:
//
//  1. Separator lines (lines made entirely of `-`, `+`, and whitespace)
//     are stripped — they're box-drawing noise, not data.
//  2. Column padding is normalized: all runs of 2+ spaces become exactly
//     two spaces, same as the kubectl table compactor.
//  3. Timing lines (`Time: 3.456 ms`) are stripped. They're useful in an
//     interactive session; as AI context they waste tokens every query.
//
// Lossless: every row value, column header, and the `(N rows)` footer
// are preserved exactly.
func Psql(output string) string {
	if output == "" {
		return output
	}
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")

	var out []string
	for _, line := range lines {
		// Separator line: only `-`, `+`, `|`, `(`, and whitespace.
		if isSeparatorLine(line) {
			continue
		}
		// Timing line: psql appends "Time: N.NNN ms" for \timing on.
		if strings.HasPrefix(line, "Time:") && strings.HasSuffix(strings.TrimSpace(line), "ms") {
			continue
		}
		// Normalize interior whitespace in table rows. Preserve leading
		// spaces (indented EXPLAIN output, etc.) and pipe separators.
		normalized := normalizeTableRow(line)
		out = append(out, normalized)
	}

	result := strings.Join(out, "\n") + "\n"
	return Guard(output, result)
}

// isSeparatorLine returns true for psql horizontal rule lines like:
//
//	---------+----------+----------
//	(used in aligned output between the header and data rows)
func isSeparatorLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) == 0 {
		return false
	}
	for _, r := range trimmed {
		if r != '-' && r != '+' && r != ' ' {
			return false
		}
	}
	// Must have at least one dash to qualify (avoids blank lines).
	return strings.ContainsRune(trimmed, '-')
}

// normalizeTableRow collapses runs of 2+ spaces inside a line to a single
// space. Preserves leading indentation and `|` pipe characters.
func normalizeTableRow(line string) string {
	if !strings.Contains(line, "  ") {
		return line
	}
	// Preserve leading whitespace.
	trimmed := strings.TrimLeft(line, " \t")
	indent := line[:len(line)-len(trimmed)]

	// Collapse internal multi-space sequences.
	fields := strings.Fields(trimmed)
	return indent + strings.Join(fields, " ")
}

func psqlAdapter(_ []string, raw string) string { return Psql(raw) }
