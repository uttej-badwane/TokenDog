package filter

import (
	"strings"
	"testing"
)

// TestFindPassThroughOnInflation is the regression test for v0.4.3's
// "find filter inflates small inputs" bug. When grouping overhead would
// make the output larger than the original, the filter must return the
// original unchanged — anything else violates the lossless contract.
func TestFindPassThroughOnInflation(t *testing.T) {
	// Small input where directory-grouping headers would exceed the savings.
	in := `cmd/cloud.go
cmd/curl.go
cmd/git.go
`
	out := Find(in)
	if len(out) > len(in) {
		t.Errorf("Find produced larger output than input: filtered=%d raw=%d\nfiltered: %q",
			len(out), len(in), out)
	}
}

func TestFindCompressesNoisyDirs(t *testing.T) {
	// Files inside a noisy dir (node_modules) collapse to a single
	// "(N files)" summary, which is the only case where the find filter
	// actually compresses meaningfully.
	var lines []string
	for i := 0; i < 50; i++ {
		lines = append(lines, "/repo/node_modules/lodash/file_"+string(rune('a'+i%26))+".js")
	}
	in := strings.Join(lines, "\n") + "\n"
	out := Find(in)
	if len(out) >= len(in) {
		t.Errorf("expected compression on noisy dirs, got filtered=%d raw=%d\noutput: %q",
			len(out), len(in), out)
	}
	if !strings.Contains(out, "node_modules") {
		t.Errorf("expected node_modules summary in output, got: %q", out)
	}
}

func TestFindEmpty(t *testing.T) {
	out := Find("")
	if out != "" {
		t.Errorf("expected empty output for empty input, got: %q", out)
	}
}
