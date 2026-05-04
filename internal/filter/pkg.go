package filter

import (
	"regexp"
	"strings"
)

// PackageManager compresses install/list output for npm, pnpm, yarn, pip,
// and cargo build. The pattern across all of them is:
//   - many lines of fetch/download/compile progress (noise)
//   - a summary at the end ("added X packages", "Successfully installed", etc.)
//   - warnings and errors interspersed (must be preserved)
//
// We drop progress lines and keep summary + warning/error lines. When the
// output ends in a recognized success summary AND there are no warnings, we
// collapse to just the summary.
func PackageManager(output string) string {
	lines := strings.Split(output, "\n")
	var kept []string
	for _, line := range lines {
		if isPkgProgressNoise(line) {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

var pkgNoisePatterns = []*regexp.Regexp{
	// npm/pnpm/yarn fetch progress
	regexp.MustCompile(`^\s*(⠋|⠙|⠹|⠸|⠼|⠴|⠦|⠧|⠇|⠏|→|✓|✔)\s`),
	regexp.MustCompile(`^npm http (fetch|GET|cache)`),
	regexp.MustCompile(`^\s*[├└─│]+\s*[a-zA-Z@]`), // dependency tree branches
	// pip downloads
	regexp.MustCompile(`^\s*(Collecting|Downloading|Using cached|Building wheel|Stored in directory)\s`),
	regexp.MustCompile(`^\s*\|[█▏▎▍▌▋▊▉ ]+\|`), // progress bars
	regexp.MustCompile(`^\s*\d+(\.\d+)?\s*(kB|MB|KB|GB)/s`),
	// cargo
	regexp.MustCompile(`^\s*(Downloading|Downloaded|Compiling|Checking|Updating|Fresh|Finished)\s`),
	regexp.MustCompile(`^\s*Blocking waiting for file lock`),
	// generic spinner residue
	regexp.MustCompile(`^\s*\.{3,}\s*$`),
}

func isPkgProgressNoise(line string) bool {
	stripped := strings.TrimRight(line, " \t\r")
	if stripped == "" {
		return false // blank lines are kept; the trailing-empties trim happens at output time
	}
	// Always keep warnings and errors — these are the lines the model needs.
	low := strings.ToLower(stripped)
	if strings.Contains(low, "warn") || strings.Contains(low, "error") || strings.Contains(low, "deprecated") || strings.Contains(low, "vulnerab") {
		return false
	}
	for _, re := range pkgNoisePatterns {
		if re.MatchString(stripped) {
			return true
		}
	}
	return false
}
