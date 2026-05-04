package filter

import "testing"

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
