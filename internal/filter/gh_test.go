package filter

import (
	"strings"
	"testing"
)

// TestGHEmptyInput is the regression test for v0.4.4's "gh empty input
// produces 1 byte" bug. ghTable must not turn an empty stream into "\n".
func TestGHEmptyInput(t *testing.T) {
	if got := GH("pr", ""); got != "" {
		t.Errorf("GH(\"pr\", \"\") = %q, want empty", got)
	}
	if got := ghTable(""); got != "" {
		t.Errorf("ghTable(\"\") = %q, want empty", got)
	}
}

func TestGHTablePassThroughOnInflation(t *testing.T) {
	// Single-row table that wouldn't shrink under whitespace collapse.
	in := "NAME  STATUS\nfoo  ok\n"
	out := ghTable(in)
	if len(out) > len(in) {
		t.Errorf("ghTable produced larger output: filtered=%d raw=%d", len(out), len(in))
	}
}

func TestGHUnknownSubcmdPassthrough(t *testing.T) {
	in := "some random output\n"
	if got := GH("auth", in); got != in {
		t.Errorf("expected unknown subcmd passes through, got: %q", got)
	}
}

// TestGHRunLogStripsRedundantPrefix is the headline test for v0.8.3's
// big win: `gh run view --log` output has `job\tstep\ttimestamp ` repeated
// on every single line. Filter dedupes job+step into a section header
// and drops the redundant prefix from content lines.
func TestGHRunLogStripsRedundantPrefix(t *testing.T) {
	in := "release\tbuild\t2026-05-06T20:03:10.8128553Z First line\n" +
		"release\tbuild\t2026-05-06T20:03:11.0000000Z Second line\n" +
		"release\tbuild\t2026-05-06T20:03:12.0000000Z Third line\n"
	out := GH("run", in)
	if len(out) >= len(in) {
		t.Errorf("expected compaction, got %d -> %d:\n%s", len(in), len(out), out)
	}
	for _, must := range []string{"First line", "Second line", "Third line", "release", "build", "2026-05-06T20:03:10"} {
		if !strings.Contains(out, must) {
			t.Errorf("lossless violation: %q missing\n%s", must, out)
		}
	}
	// job\tstep\ttimestamp prefix should NOT appear on each content line.
	if strings.Count(out, "release\tbuild") > 0 {
		t.Errorf("redundant job/step prefix not stripped:\n%s", out)
	}
}

// TestGHRunLogJobBoundary tests that switching to a new job emits a fresh
// header rather than continuing under the previous one.
func TestGHRunLogJobBoundary(t *testing.T) {
	in := "build\tcompile\t2026-05-06T20:00:00.0000000Z compiling\n" +
		"build\tcompile\t2026-05-06T20:00:01.0000000Z done\n" +
		"test\trun\t2026-05-06T20:00:02.0000000Z testing\n"
	out := GH("run", in)
	if !strings.Contains(out, "=== build / compile") {
		t.Errorf("first job header missing:\n%s", out)
	}
	if !strings.Contains(out, "=== test / run") {
		t.Errorf("second job header missing:\n%s", out)
	}
}

// TestGHRunLogBOMOnFirstLine — gh embeds a UTF-8 BOM (U+FEFF) inside
// the third column of the first line. Detection must strip it.
func TestGHRunLogBOMOnFirstLine(t *testing.T) {
	in := "release\tbuild\t\uFEFF2026-05-06T20:03:10.8128553Z First line\n" +
		"release\tbuild\t2026-05-06T20:03:11.0000000Z Second line\n"
	out := GH("run", in)
	if len(out) >= len(in) {
		t.Errorf("BOM-prefixed log not detected, no compaction:\n%s", out)
	}
}

// TestGHRunLogPassthroughForNonLogContent — ghRunLog detection should
// reject non-log subcommands so `gh pr view`, `gh issue view`, etc. don't
// accidentally trigger it.
func TestGHRunLogPassthroughForNonLogContent(t *testing.T) {
	in := "this is some\nprose content\nwith no tabs\n"
	if got := GH("pr", in); got != in && len(got) > len(in) {
		t.Errorf("prose content should pass through, got: %q", got)
	}
}
