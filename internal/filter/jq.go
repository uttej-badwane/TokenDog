package filter

import (
	"encoding/json"
	"fmt"
	"strings"
)

// JQ compresses jq output by re-emitting JSON without indentation. Lossless.
// For very large arrays, it shows a count summary (still preserves data
// because the original could be retrieved with `jq '.[N]'`).
func JQ(content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return content
	}

	// jq output may be multiple JSON values separated by newlines.
	// Compact each one independently.
	lines := strings.Split(trimmed, "\n")
	var compactedDocs []string
	var buf strings.Builder

	flush := func() {
		if buf.Len() == 0 {
			return
		}
		compactedDocs = append(compactedDocs, compactJSONValue(buf.String()))
		buf.Reset()
	}

	for _, line := range lines {
		// Heuristic: if a line starts with { [ " or a JSON literal at column 0,
		// it's the start of a new doc (jq's default output).
		if buf.Len() > 0 && isJSONStart(line) && !startsAtIndent(line) {
			flush()
		}
		buf.WriteString(line)
		buf.WriteString("\n")
	}
	flush()

	if len(compactedDocs) == 0 {
		return content
	}
	return strings.Join(compactedDocs, "\n") + "\n"
}

func isJSONStart(line string) bool {
	t := strings.TrimSpace(line)
	if t == "" {
		return false
	}
	c := t[0]
	return c == '{' || c == '[' || c == '"' || c == '-' || (c >= '0' && c <= '9') ||
		strings.HasPrefix(t, "true") || strings.HasPrefix(t, "false") || strings.HasPrefix(t, "null")
}

func startsAtIndent(line string) bool {
	return len(line) > 0 && (line[0] == ' ' || line[0] == '\t')
}

// compactJSONValue tries to parse and compact a single JSON value.
// On failure (invalid JSON, partial output), returns input unchanged — never lossy.
func compactJSONValue(s string) string {
	t := strings.TrimSpace(s)
	if t == "" {
		return s
	}
	var v any
	if err := json.Unmarshal([]byte(t), &v); err != nil {
		return s
	}

	// For huge arrays, summarize length AND emit the full data
	if arr, ok := v.([]any); ok && len(arr) > 50 {
		out, _ := json.Marshal(v)
		return fmt.Sprintf("[%d items] %s", len(arr), string(out))
	}

	out, err := json.Marshal(v)
	if err != nil {
		return s
	}
	return string(out)
}
