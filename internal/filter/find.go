package filter

import (
	"fmt"
	"sort"
	"strings"
)

var noisyDirs = []string{
	"node_modules", ".git", "vendor", ".next",
	"dist", "build", "__pycache__", "target", ".cache",
}

func Find(output string) string {
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")

	var paths []string
	for _, line := range lines {
		if t := strings.TrimSpace(line); t != "" && t != "." {
			paths = append(paths, t)
		}
	}
	if len(paths) == 0 {
		return output
	}

	var clean, noisy []string
	for _, p := range paths {
		if isNoisyPath(p) {
			noisy = append(noisy, p)
		} else {
			clean = append(clean, p)
		}
	}

	// Group all results — no cap, grouping is the compression
	grouped := groupPaths(clean)
	dirKeys := make([]string, 0, len(grouped))
	for d := range grouped {
		dirKeys = append(dirKeys, d)
	}
	sort.Strings(dirKeys)

	var sb strings.Builder
	for _, dir := range dirKeys {
		files := grouped[dir]
		if dir == "." {
			for _, f := range files {
				sb.WriteString(f + "\n")
			}
		} else if len(files) == 1 {
			sb.WriteString(files[0] + "\n")
		} else {
			// List all files under this dir, indented
			sb.WriteString(fmt.Sprintf("%s/ (%d files)\n", dir, len(files)))
			for _, f := range files {
				sb.WriteString(fmt.Sprintf("  %s\n", f))
			}
		}
	}

	// Noisy dirs: show count only — their contents are almost never useful
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

	filtered := sb.String()
	if len(filtered) >= len(output) {
		return output
	}
	return filtered
}

func isNoisyPath(path string) bool {
	for _, dir := range noisyDirs {
		if strings.Contains("/"+path+"/", "/"+dir+"/") {
			return true
		}
	}
	return false
}

func noisyRoot(path string) string {
	for _, dir := range noisyDirs {
		if idx := strings.Index(path, dir); idx >= 0 {
			return path[:idx+len(dir)]
		}
	}
	return path
}

func groupPaths(paths []string) map[string][]string {
	groups := map[string][]string{}
	for _, p := range paths {
		idx := strings.LastIndex(p, "/")
		dir := "."
		if idx > 0 {
			dir = p[:idx]
		}
		groups[dir] = append(groups[dir], p)
	}
	return groups
}
