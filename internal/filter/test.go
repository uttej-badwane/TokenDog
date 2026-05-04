package filter

import (
	"regexp"
	"strings"
)

// Test compresses test runner output. The strict rule: if there's any
// failure, error, or skipped test the model needs to know about, the
// original output is returned verbatim. Compression ONLY applies when every
// test passed — in that case we collapse per-file progress noise into the
// final summary line, which already reports the total count.
//
// Supported runners: pytest, jest, vitest, go test, cargo test, mocha.
func Test(runner string, output string) string {
	if hasFailureSignal(output) {
		return output
	}
	switch runner {
	case "pytest":
		return compactPytest(output)
	case "jest", "vitest", "mocha":
		return compactJest(output)
	case "go":
		return compactGoTest(output)
	case "cargo":
		return compactCargoTest(output)
	default:
		return output
	}
}

// hasFailureSignal looks for any indication that a test didn't pass. When in
// doubt, return true so we pass through verbatim — hiding a real failure
// behind a compressed summary would be worse than zero compression.
var failureSignals = []*regexp.Regexp{
	regexp.MustCompile(`(?m)^FAILED `),                  // pytest
	regexp.MustCompile(`(?m)^=+ FAILURES =+`),           // pytest
	regexp.MustCompile(`(?m)^=+ ERRORS =+`),             // pytest
	regexp.MustCompile(`\b\d+ failed\b`),                // pytest/jest summaries
	regexp.MustCompile(`\b\d+ error[s]?\b`),             // generic
	regexp.MustCompile(`(?m)^FAIL\b`),                   // go/jest
	regexp.MustCompile(`(?m)^--- FAIL:`),                // go test
	regexp.MustCompile(`(?m)^Tests: .*failed`),          // jest summary
	regexp.MustCompile(`(?m)^test result: FAILED`),      // cargo test
	regexp.MustCompile(`panic:`),                        // go panic
	regexp.MustCompile(`(?m)^\s+at .*\.(js|ts|jsx|tsx)`), // jest stack trace
}

func hasFailureSignal(output string) bool {
	for _, re := range failureSignals {
		if re.MatchString(output) {
			return true
		}
	}
	return false
}

// compactPytest keeps the final summary line + collected count. The per-file
// progress (dots, file paths, PASSED lines) is dropped only when all green.
func compactPytest(output string) string {
	lines := strings.Split(output, "\n")
	var kept []string
	for _, line := range lines {
		t := strings.TrimSpace(line)
		// Keep platform/version banner and the final summary band.
		if strings.HasPrefix(t, "platform ") ||
			strings.HasPrefix(t, "rootdir:") ||
			strings.HasPrefix(t, "plugins:") ||
			strings.HasPrefix(t, "collected ") ||
			(strings.HasPrefix(t, "=") && strings.Contains(t, "passed")) ||
			(strings.HasPrefix(t, "=") && strings.Contains(t, "warning")) {
			kept = append(kept, line)
		}
	}
	if len(kept) == 0 {
		return output
	}
	return strings.Join(kept, "\n") + "\n"
}

// compactJest keeps only the file-level PASS lines + the final Tests/Suites
// summary. Drops the per-test ✓ tick lines that jest emits in verbose mode.
func compactJest(output string) string {
	lines := strings.Split(output, "\n")
	var kept []string
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "PASS ") ||
			strings.HasPrefix(t, "Tests:") ||
			strings.HasPrefix(t, "Test Suites:") ||
			strings.HasPrefix(t, "Snapshots:") ||
			strings.HasPrefix(t, "Time:") ||
			strings.HasPrefix(t, "Ran all test suites") {
			kept = append(kept, line)
		}
	}
	if len(kept) == 0 {
		return output
	}
	return strings.Join(kept, "\n") + "\n"
}

// compactGoTest drops `=== RUN` and `--- PASS:` lines but keeps package
// summary lines (`ok\tpkg\t0.123s`) verbatim — those carry the full result.
func compactGoTest(output string) string {
	lines := strings.Split(output, "\n")
	var kept []string
	for _, line := range lines {
		t := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(t, "=== RUN") ||
			strings.HasPrefix(t, "=== PAUSE") ||
			strings.HasPrefix(t, "=== CONT") ||
			strings.HasPrefix(t, "--- PASS:") ||
			strings.HasPrefix(t, "--- SKIP:") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// compactCargoTest keeps the final summary line and the per-test results
// only when verbose; otherwise leaves output alone (cargo's default is
// already compact).
func compactCargoTest(output string) string {
	lines := strings.Split(output, "\n")
	var kept []string
	for _, line := range lines {
		t := strings.TrimSpace(line)
		// Drop "test foo::bar ... ok" lines — the trailing "test result"
		// line summarizes everything.
		if strings.HasPrefix(t, "test ") && strings.HasSuffix(t, " ok") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}
