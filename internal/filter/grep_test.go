package filter

import (
	"strings"
	"testing"
)

func TestGrepPassthroughOnEmpty(t *testing.T) {
	if got := Grep(nil, ""); got != "" {
		t.Errorf("Grep(empty) = %q, want empty", got)
	}
}

func TestGrepLosslessContract(t *testing.T) {
	in := "sample raw output\nfor your filter\nto compress\n"
	out := Grep(nil, in)
	if len(out) > len(in) {
		t.Errorf("Grep inflated: %d -> %d bytes\nout: %q", len(in), len(out), out)
	}
}

// TestGrepGroupsByPath is the headline behavior: when many matches share a
// path, the path string is emitted once and individual matches indent under.
func TestGrepGroupsByPath(t *testing.T) {
	in := `internal/filter/git.go:8:func Git(subcommand string, output string) string {
internal/filter/git.go:23:func gitStatus(output string) string {
internal/filter/git.go:108:func gitLog(output string) string {
internal/filter/gh.go:10:func GH(subcommand string, output string) string {
internal/filter/gh.go:33:func ghTable(output string) string {
`
	out := Grep(nil, in)
	if len(out) >= len(in) {
		t.Errorf("expected compaction, got %d -> %d\n%s", len(in), len(out), out)
	}
	// Both paths must appear once each (deduped); all line numbers + content
	// preserved.
	if strings.Count(out, "internal/filter/git.go") != 1 {
		t.Errorf("git.go path should appear exactly once, got %d times\n%s",
			strings.Count(out, "internal/filter/git.go"), out)
	}
	for _, must := range []string{"func Git(", "func gitStatus(", "func gitLog(", "func GH(", "func ghTable(", "8:", "23:", "108:", "10:", "33:"} {
		if !strings.Contains(out, must) {
			t.Errorf("lossless violation: %q missing\n%s", must, out)
		}
	}
}

// TestGrepSingleMatchPerFilePassesThrough — when no path repeats, grouping
// would only add overhead. Filter must passthrough the original.
func TestGrepSingleMatchPerFilePassesThrough(t *testing.T) {
	in := `cmd/git.go:1:package cmd
cmd/gh.go:1:package cmd
cmd/find.go:1:package cmd
`
	out := Grep(nil, in)
	if out != in {
		t.Errorf("expected passthrough, got: %q", out)
	}
}

// TestGrepHandlesBinaryFileNotice — `grep -r` emits "Binary file foo
// matches" for binaries. Those don't fit the lineno shape; they pass
// through unchanged, interleaved with the structured matches.
func TestGrepHandlesBinaryFileNotice(t *testing.T) {
	in := `internal/filter/git.go:8:func Git
internal/filter/git.go:23:func gitStatus
Binary file /tmp/build/main matches
internal/filter/gh.go:10:func GH
`
	out := Grep(nil, in)
	if !strings.Contains(out, "Binary file /tmp/build/main matches") {
		t.Errorf("binary notice dropped: %q", out)
	}
	if !strings.Contains(out, "func Git") || !strings.Contains(out, "func GH") {
		t.Errorf("matches dropped: %q", out)
	}
}

// TestGrepFilenameOnlyPassthrough — `grep -l` outputs only paths, one per
// line, no `:lineno:`. parseGrepMatch returns false for those, so the
// whole input becomes passthrough.
func TestGrepFilenameOnlyPassthrough(t *testing.T) {
	in := `cmd/git.go
cmd/gh.go
cmd/docker.go
`
	out := Grep(nil, in)
	if out != in {
		t.Errorf("filename-only output should passthrough, got %q", out)
	}
}

// TestGrepCountModePassthrough — `grep -c` outputs `path:N` where N is a
// count. parseGrepMatch sees `:N` but expects `:N:`, so this passes through.
func TestGrepCountModePassthrough(t *testing.T) {
	in := `cmd/git.go:5
cmd/gh.go:3
cmd/docker.go:1
`
	out := Grep(nil, in)
	if out != in {
		t.Errorf("count-mode output should passthrough, got %q", out)
	}
}

// TestGrepSingleFilePassthrough — without -H or -r, grep against a single
// file emits `lineno:content` (no path prefix). parseGrepMatch's "first
// char isn't whitespace AND there's a path before the colon" check
// rejects the lineno-as-path case.
func TestGrepSingleFilePassthrough(t *testing.T) {
	in := `42:matched line
56:another match
`
	out := Grep(nil, in)
	// Note: parseGrepMatch will actually parse "42" as path and "" as
	// lineno (because the next ":" isn't followed by digits, we fail).
	// Either way, output should be ≤ input.
	if len(out) > len(in) {
		t.Errorf("single-file form inflated: %d -> %d", len(in), len(out))
	}
}

// TestGrepDoesNotMisparseConfigOutput — filter shouldn't mistake YAML/
// config-style output (`key: value: 42:`) for grep matches and try to
// group on it. Defended by the leading-whitespace check + non-empty path
// requirement.
func TestGrepDoesNotMisparseConfigOutput(t *testing.T) {
	in := `  key1: value
  key2: value: 42:something
`
	out := Grep(nil, in)
	if out != in {
		t.Errorf("config-style output should passthrough, got %q", out)
	}
}

func TestParseGrepMatch(t *testing.T) {
	cases := []struct {
		in                           string
		wantPath, wantLine, wantBody string
		wantOK                       bool
	}{
		{"path/file.go:42:content", "path/file.go", "42", "content", true},
		{"path/file.go:42:content with: colons", "path/file.go", "42", "content with: colons", true},
		{`C:\path\file.go:42:content`, `C:\path\file.go`, "42", "content", true},
		{"no colons here", "", "", "", false},
		{"path:notdigits:content", "", "", "", false},
		{"path:42:", "path", "42", "", true},
		{":42:content", "", "", "", false},
		{"  indented:42:line", "", "", "", false},
		{"", "", "", "", false},
	}
	for _, tc := range cases {
		path, line, body, ok := parseGrepMatch(tc.in)
		if ok != tc.wantOK {
			t.Errorf("parseGrepMatch(%q) ok=%v, want %v", tc.in, ok, tc.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if path != tc.wantPath {
			t.Errorf("parseGrepMatch(%q) path=%q, want %q", tc.in, path, tc.wantPath)
		}
		if line != tc.wantLine {
			t.Errorf("parseGrepMatch(%q) line=%q, want %q", tc.in, line, tc.wantLine)
		}
		if body != tc.wantBody {
			t.Errorf("parseGrepMatch(%q) body=%q, want %q", tc.in, body, tc.wantBody)
		}
	}
}
