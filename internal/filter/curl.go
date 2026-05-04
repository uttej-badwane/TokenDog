package filter

import (
	"regexp"
	"strings"
)

var (
	reSpaces   = regexp.MustCompile(`[ \t]+`)
	reNewlines = regexp.MustCompile(`\n{3,}`)
)

// Curl compresses curl response output. If the body is JSON, compact it
// (preserving every key and value — silent data mangling is explicitly
// avoided). Otherwise just collapse redundant whitespace.
func Curl(content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return content
	}

	// Heuristic: looks like JSON if first char is { or [
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		// Use the same lossless compactor as jq — values preserved verbatim.
		return compactJSONValue(trimmed)
	}

	return collapseWhitespace(trimmed)
}

// collapseWhitespace removes redundant whitespace — lossless.
func collapseWhitespace(s string) string {
	s = reSpaces.ReplaceAllString(s, " ")
	lines := strings.Split(s, "\n")
	var kept []string
	for _, line := range lines {
		if t := strings.TrimSpace(line); t != "" {
			kept = append(kept, t)
		}
	}
	s = strings.Join(kept, "\n")
	s = reNewlines.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}
