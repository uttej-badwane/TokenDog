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
