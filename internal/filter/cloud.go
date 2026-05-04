package filter

import "strings"

// Cloud compresses aws/gcloud/az output. The output shape varies by command:
//   - JSON (most aws describe/list defaults) — compacted losslessly via JSON
//     re-marshal. Every field is preserved.
//   - Table — column whitespace normalized.
//   - YAML — blank lines collapsed.
//   - Plain text (s3 ls) — passed through.
func Cloud(output string) string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return output
	}

	// JSON: most aws/gcloud commands default to JSON (aws) or YAML (gcloud).
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return compactJSONValue(trimmed)
	}

	// Heuristic for table output: starts with a column header line followed
	// by 2+ space gaps. We use the same gh/kubectl table compactor.
	if looksLikeTable(output) {
		return ghTable(output)
	}

	// YAML or plain text: collapse runs of blank lines.
	return collapseBlankLines(output)
}

func looksLikeTable(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		if looksTabular(line) {
			return true
		}
	}
	return false
}

func collapseBlankLines(output string) string {
	lines := strings.Split(output, "\n")
	var kept []string
	blankRun := 0
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			blankRun++
			if blankRun > 1 {
				continue
			}
		} else {
			blankRun = 0
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}
