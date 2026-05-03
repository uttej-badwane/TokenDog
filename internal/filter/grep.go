package filter

import (
	"fmt"
	"sort"
	"strings"
)

// Grep compresses Grep tool output by grouping matches under their file.
// Input format: "path/to/file:lineno:matched content" per line.
// Lossless — every match is preserved, just structured by file.
func Grep(content string) string {
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if len(lines) == 0 {
		return content
	}

	type match struct {
		line    string
		content string
	}
	byFile := map[string][]match{}
	var fileOrder []string
	var orphans []string

	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 3 {
			orphans = append(orphans, line)
			continue
		}
		file, lineno, body := parts[0], parts[1], parts[2]
		if _, ok := byFile[file]; !ok {
			fileOrder = append(fileOrder, file)
		}
		byFile[file] = append(byFile[file], match{line: lineno, content: strings.TrimSpace(body)})
	}

	if len(byFile) == 0 {
		return content
	}

	sort.Strings(fileOrder)

	var sb strings.Builder
	for _, file := range fileOrder {
		matches := byFile[file]
		if len(matches) == 1 {
			sb.WriteString(fmt.Sprintf("%s:%s: %s\n", file, matches[0].line, matches[0].content))
			continue
		}
		sb.WriteString(fmt.Sprintf("%s (%d matches)\n", file, len(matches)))
		for _, m := range matches {
			sb.WriteString(fmt.Sprintf("  %s: %s\n", m.line, m.content))
		}
	}
	for _, o := range orphans {
		sb.WriteString(o + "\n")
	}
	return sb.String()
}
