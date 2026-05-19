package filter

import "strings"

// Cat compresses `cat`, `head`, and `tail` output. File contents must
// never be structurally altered — code, config, and data are all equally
// valid and lose meaning if reordered or truncated. The only safe
// transformations are:
//
//   - Collapse runs of 3+ consecutive blank lines to 2. Long blank
//     stretches add no information.
//   - Strip trailing whitespace per line. Never affects code behaviour
//     and saves a meaningful number of tokens in generated or IDE-edited
//     files.
//
// The filter activates only when the output is large enough that the
// blank-line pass can pay for itself — for small files the passthrough
// cost is ~0 anyway.
func Cat(output string) string {
	if output == "" {
		return output
	}
	lines := strings.Split(output, "\n")
	var out []string
	blankRun := 0
	for _, line := range lines {
		stripped := strings.TrimRight(line, " \t")
		if stripped == "" {
			blankRun++
			if blankRun > 2 {
				continue
			}
		} else {
			blankRun = 0
		}
		out = append(out, stripped)
	}
	return Guard(output, strings.Join(out, "\n"))
}

func catAdapter(_ []string, raw string) string { return Cat(raw) }
