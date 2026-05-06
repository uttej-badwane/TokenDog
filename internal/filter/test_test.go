package filter

import (
	"strings"
	"testing"
)

// TestFilterPassingPytestCollapsesToSummary is the headline behavior: a
// successful pytest run with all-green should drop to just the summary.
// On any failure marker, output must be passed through verbatim — never
// hide failures.
func TestFilterPassingPytestCollapsesToSummary(t *testing.T) {
	in := `============= test session starts ==============
platform darwin -- Python 3.11
collected 50 items

test_one.py ........................ [ 48%]
test_two.py ........................ [ 96%]
test_three.py ..                     [100%]

============= 50 passed in 1.42s ==============
`
	out := Test("pytest", in)
	if !strings.Contains(out, "50 passed") {
		t.Errorf("summary lost: %q", out)
	}
	if len(out) >= len(in) {
		t.Errorf("expected collapse, got %d -> %d", len(in), len(out))
	}
}

func TestFilterFailingPytestPassesThrough(t *testing.T) {
	in := `============= test session starts ==============
test_x.py F                                       [100%]

================== FAILURES ==================
______________________ test_x ________________________
    assert 1 == 2
E   AssertionError: assert 1 == 2

============= 1 failed in 0.5s ==============
`
	out := Test("pytest", in)
	if !strings.Contains(out, "FAILURES") || !strings.Contains(out, "AssertionError") {
		t.Errorf("failure body must pass through verbatim, got: %q", out)
	}
}

func TestFilterPassingGoTestCollapses(t *testing.T) {
	in := `=== RUN   TestFoo
--- PASS: TestFoo (0.00s)
=== RUN   TestBar
--- PASS: TestBar (0.01s)
=== RUN   TestBaz
--- PASS: TestBaz (0.00s)
PASS
ok  	example.com/m	0.123s
`
	out := Test("go", in)
	if !strings.Contains(out, "PASS") || !strings.Contains(out, "example.com/m") {
		t.Errorf("summary missing: %q", out)
	}
	if len(out) >= len(in) {
		t.Errorf("expected collapse, got %d -> %d", len(in), len(out))
	}
}

func TestFilterFailingGoTestPassesThrough(t *testing.T) {
	in := `=== RUN   TestFoo
    foo_test.go:10: expected 1 got 2
--- FAIL: TestFoo (0.00s)
FAIL
exit status 1
`
	out := Test("go", in)
	if !strings.Contains(out, "FAIL") || !strings.Contains(out, "expected 1 got 2") {
		t.Errorf("failure must pass through, got: %q", out)
	}
}

func TestFilterEmptyInput(t *testing.T) {
	for _, runner := range []string{"pytest", "jest", "vitest", "go", "cargo"} {
		if got := Test(runner, ""); got != "" {
			t.Errorf("Test(%q, empty) = %q, want empty", runner, got)
		}
	}
}

func TestFilterUnknownRunner(t *testing.T) {
	in := "anything\nat all\n"
	out := Test("unknown-runner", in)
	if out != in {
		t.Errorf("unknown runner should passthrough: %q", out)
	}
}
