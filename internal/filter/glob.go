package filter

import (
	"fmt"
	"sort"
	"strings"
)

// Glob compresses Glob tool output by grouping paths under their parent
// directory. Lossless — every path is still represented, just structured.
func Glob(content string) string {
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")

	var paths []string
	for _, line := range lines {
		if t := strings.TrimSpace(line); t != "" {
			paths = append(paths, t)
		}
	}
	if len(paths) == 0 {
		return content
	}

	var clean, noisy []string
	for _, p := range paths {
		if isNoisyPath(p) {
			noisy = append(noisy, p)
		} else {
			clean = append(clean, p)
		}
	}

	grouped := groupPaths(clean)
	dirKeys := make([]string, 0, len(grouped))
	for d := range grouped {
		dirKeys = append(dirKeys, d)
	}
	sort.Strings(dirKeys)

	var sb strings.Builder
	for _, dir := range dirKeys {
		files := grouped[dir]
		switch {
		case dir == ".":
			for _, f := range files {
				sb.WriteString(f + "\n")
			}
		case len(files) == 1:
			sb.WriteString(files[0] + "\n")
		default:
			sb.WriteString(fmt.Sprintf("%s/ (%d files)\n", dir, len(files)))
			for _, f := range files {
				sb.WriteString("  " + f + "\n")
			}
		}
	}

	if len(noisy) > 0 {
		counts := map[string]int{}
		for _, p := range noisy {
			counts[noisyRoot(p)]++
		}
		roots := make([]string, 0, len(counts))
		for r := range counts {
			roots = append(roots, r)
		}
		sort.Strings(roots)
		for _, r := range roots {
			sb.WriteString(fmt.Sprintf("%s/ (%d files — use explicit path to explore)\n", r, counts[r]))
		}
	}

	return sb.String()
}
