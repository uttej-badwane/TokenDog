package filter

import (
	"strings"
	"testing"
)

// TestMakeStripsPrettyCompileSpam targets the pretty-printed output that
// many Makefiles emit ("  CC main.o", "  CXX widget.o"). Make.go's
// short-label regex catches these. Raw `cc -c -O2 ...` lines are NOT
// stripped — the filter is conservative: only lines clearly matching the
// build-system convention get dropped.
func TestMakeStripsPrettyCompileSpam(t *testing.T) {
	in := `  CC src/main.o
  CC src/util.o
src/util.c:42:5: warning: unused variable 'foo'
  CC src/foo.o
  LD foo
`
	out := Make(in)
	if !strings.Contains(out, "warning: unused variable") {
		t.Errorf("warning dropped: %q", out)
	}
	if len(out) >= len(in) {
		t.Errorf("expected compile-line stripping, got %d -> %d", len(in), len(out))
	}
}

// TestMakeStripsNinjaProgress tests another supported pattern.
func TestMakeStripsNinjaProgress(t *testing.T) {
	in := `[1/100] Building CXX object src/foo.o
[2/100] Building CXX object src/bar.o
src/bar.cpp:10: error: undefined reference to 'frobnicate'
[3/100] Linking CXX executable target
`
	out := Make(in)
	if !strings.Contains(out, "error: undefined reference") {
		t.Errorf("error dropped: %q", out)
	}
	if len(out) >= len(in) {
		t.Errorf("expected ninja-progress stripping, got %d -> %d", len(in), len(out))
	}
}

func TestMakeKeepsErrorsVerbatim(t *testing.T) {
	in := `cc -c src/main.c -o main.o
src/main.c:10:1: error: undefined reference to 'foo'
make: *** [main] Error 1
`
	out := Make(in)
	for _, must := range []string{"error: undefined reference", "Error 1"} {
		if !strings.Contains(out, must) {
			t.Errorf("error/diagnostic dropped: %q\nfull: %s", must, out)
		}
	}
}

func TestMakeEmpty(t *testing.T) {
	if got := Make(""); got != "" {
		t.Errorf("empty -> empty, got %q", got)
	}
}
