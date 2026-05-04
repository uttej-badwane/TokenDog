package filter

import (
	"regexp"
	"strings"
)

// Make compresses make/cmake/ninja output. The pattern: many `[CC] foo.o`
// or `cc -I... -W... foo.c -o foo.o` lines that are noise once they
// succeed, plus the warnings/errors the model needs verbatim. We drop
// successful compile lines and keep everything else.
//
// Lossless guarantee: any line containing "warning", "error", "undefined",
// or "fatal" is preserved unchanged. If a build fails, the failing command
// line is preserved (we only strip lines that match the success pattern).
func Make(output string) string {
	lines := strings.Split(output, "\n")
	var kept []string
	for _, line := range lines {
		if isMakeNoise(line) {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

var makeNoisePatterns = []*regexp.Regexp{
	// `make[1]: Entering directory ...` / `Leaving directory`
	regexp.MustCompile(`^make\[\d+\]: (Entering|Leaving) directory`),
	regexp.MustCompile(`^make: (Entering|Leaving) directory`),
	// ninja progress: `[12/345] Building CXX object ...`
	regexp.MustCompile(`^\[\d+/\d+\] (Building|Linking|Generating|Compiling)\s`),
	// cmake configure spam
	regexp.MustCompile(`^-- (Looking for|Performing|Check for|Detecting|Found|Configuring done|Generating done|Build files have been written)`),
	// short labels emitted in pretty output: `  CC foo.o`, `  LINK target`
	regexp.MustCompile(`^\s*(CC|CXX|LD|AR|RANLIB|LINK|GEN|CCLD|INSTALL)\s+\S`),
}

func isMakeNoise(line string) bool {
	stripped := strings.TrimRight(line, " \t\r")
	if stripped == "" {
		return false
	}
	low := strings.ToLower(stripped)
	if strings.Contains(low, "warning") || strings.Contains(low, "error") ||
		strings.Contains(low, "undefined") || strings.Contains(low, "fatal") {
		return false
	}
	for _, re := range makeNoisePatterns {
		if re.MatchString(stripped) {
			return true
		}
	}
	return false
}
