package filter

import "strings"

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
