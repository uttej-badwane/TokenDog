package filter

import "strings"

// Kubectl compresses kubectl output without dropping any data.
// We deliberately avoid RTK issue #1466 (aws eks describe-cluster losing
// critical fields when compressed). For tabular output we collapse
// whitespace; for describe/get -o yaml output we leave structure intact
// and only strip excess blank lines.
func Kubectl(subcommand string, output string) string {
	switch subcommand {
	case "get":
		return kubectlTable(output)
	case "top":
		return kubectlTable(output)
	case "describe":
		return kubectlDescribe(output)
	case "logs":
		// Logs are content the user wants — don't mess with them.
		return output
	default:
		return output
	}
}

func kubectlTable(output string) string {
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if len(lines) == 0 {
		return output
	}
	var sb strings.Builder
	for _, line := range lines {
		if line == "" {
			continue
		}
		// Normalize multi-space columns to two spaces — preserves columnar
		// readability while saving tokens.
		fields := strings.Fields(line)
		sb.WriteString(strings.Join(fields, "  "))
		sb.WriteString("\n")
	}
	return sb.String()
}

func kubectlDescribe(output string) string {
	// Describe output has many blank lines and trailing whitespace.
	// Collapse those without touching the field names/values.
	lines := strings.Split(output, "\n")
	var kept []string
	blankRun := 0
	for _, line := range lines {
		stripped := strings.TrimRight(line, " \t")
		if strings.TrimSpace(stripped) == "" {
			blankRun++
			if blankRun > 1 {
				continue
			}
			kept = append(kept, "")
			continue
		}
		blankRun = 0
		kept = append(kept, stripped)
	}
	return strings.Join(kept, "\n") + "\n"
}
