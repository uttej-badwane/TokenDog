package filter

import (
	"strings"
	"testing"
)

// TestLsPreservesDotfiles is the regression test for v0.4.4's
// "ls drops every dotfile silently" bug. When a user runs `ls -la`,
// they explicitly asked for dotfiles — silently dropping them is a
// lossless-contract violation.
func TestLsPreservesDotfiles(t *testing.T) {
	in := `total 24
drwxr-xr-x  4 user staff  128 May  4 11:22 .
drwxr-xr-x 10 user staff  320 May  4 11:14 ..
-rw-r--r--  1 user staff  100 May  4 11:14 .hidden
-rw-r--r--  1 user staff  200 May  4 11:14 visible
`
	out := Ls(in)
	if !strings.Contains(out, ".hidden") {
		t.Errorf("expected .hidden preserved, got: %q", out)
	}
	if !strings.Contains(out, "visible") {
		t.Errorf("expected visible preserved, got: %q", out)
	}
	// "." and ".." themselves are still skipped (they carry no useful info).
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "." || trimmed == ".." {
			t.Errorf("expected . and .. skipped, found in output: %q", out)
		}
	}
}

// TestLsNonLongFormatPassthrough is the regression test for v0.5.0's
// "ls panic on plain ls output" bug. `td replay` fed historical plain `ls`
// output (no -l) into the filter; lines with 9+ filenames separated by
// spaces tripped the field-count heuristic and panicked at perms[3:]. The
// fix: if fields[0] doesn't look like a 10-char permission string, treat
// the whole input as not-long-format and pass through unchanged.
func TestLsNonLongFormatPassthrough(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Ls panicked on plain `ls` output: %v", r)
		}
	}()
	in := "a b c d e f g h i j k l\nfoo bar baz qux quux corge grault garply waldo\n"
	out := Ls(in)
	if out == "" {
		t.Errorf("expected non-empty output (passthrough), got empty")
	}
}

func TestLsCompactsLongFormat(t *testing.T) {
	in := `total 24
drwxr-xr-x  4 user staff  128 May  4 11:22 src
-rw-r--r--  1 user staff 1024 May  4 11:14 README.md
`
	out := Ls(in)
	if !strings.Contains(out, "src/") {
		t.Errorf("expected dir marker '/' on src, got: %q", out)
	}
	if !strings.Contains(out, "README.md") {
		t.Errorf("expected README.md in output, got: %q", out)
	}
	if len(out) >= len(in) {
		t.Errorf("expected ls output to compress (got %d, raw %d)", len(out), len(in))
	}
}
