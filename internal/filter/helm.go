package filter

import "strings"

// Helm compresses helm output by subcommand.
//
//   - list/ls: table normalization (collapse column padding to 2 spaces,
//     strip the wide UPDATED timestamp down to a date).
//   - status: collapse blank lines, strip trailing whitespace.
//   - diff: preserve verbatim — diffs are already dense and meaningful.
//   - history: table normalization.
//   - Everything else: passthrough.
func Helm(subcommand string, output string) string {
	switch subcommand {
	case "list", "ls", "history":
		return helmTable(output)
	case "status":
		return helmStatus(output)
	case "diff":
		return output
	default:
		return output
	}
}

// helmTable normalises a helm tabular output (helm list / helm history).
// Each column is separated by 2+ spaces. We collapse those to exactly two
// spaces so the table is still readable but wastes no padding tokens.
// The UPDATED column in helm list contains a full timestamp with timezone
// and nanoseconds; we trim that down to a date+time to save ~20 chars per row.
func helmTable(output string) string {
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if len(lines) == 0 {
		return output
	}
	var sb strings.Builder
	for _, line := range lines {
		if line == "" {
			continue
		}
		// Normalize multi-space padding to exactly two spaces.
		fields := strings.Fields(line)
		// Shorten timestamps: "2024-05-01 12:00:00.123456789 +0000 UTC"
		// → "2024-05-01 12:00:00"
		for i, f := range fields {
			if len(f) == 10 && f[4] == '-' && f[7] == '-' {
				// Looks like a date field; the next two fields may be
				// time and timezone — merge and truncate.
				if i+2 < len(fields) && strings.Contains(fields[i+1], ":") {
					timeStr := fields[i+1]
					if dot := strings.Index(timeStr, "."); dot >= 0 {
						timeStr = timeStr[:dot]
					}
					fields[i] = f + " " + timeStr
					fields = append(fields[:i+1], fields[i+3:]...)
				}
			}
		}
		sb.WriteString(strings.Join(fields, "  "))
		sb.WriteString("\n")
	}
	return Guard(output, sb.String())
}

// helmStatus collapses excess blank lines and strips trailing whitespace.
// The key/value pairs in status output are already terse; we just clean up
// the vertical whitespace that helm often adds between sections.
func helmStatus(output string) string {
	lines := strings.Split(output, "\n")
	var out []string
	blankRun := 0
	for _, line := range lines {
		stripped := strings.TrimRight(line, " \t")
		if strings.TrimSpace(stripped) == "" {
			blankRun++
			if blankRun > 1 {
				continue
			}
			out = append(out, "")
			continue
		}
		blankRun = 0
		out = append(out, stripped)
	}
	return Guard(output, strings.Join(out, "\n")+"\n")
}

func helmAdapter(args []string, raw string) string {
	return Helm(ExtractSubcmd(args, nil), raw)
}
