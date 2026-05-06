package filter

import (
	"strings"
	"testing"
)

// TestPackageManagerStripsPipProgress targets pip's recognized progress
// lines (Collecting, Downloading, Building wheel). Warnings preserved.
func TestPackageManagerStripsPipProgress(t *testing.T) {
	in := `Collecting requests==2.31.0
  Downloading requests-2.31.0-py3-none-any.whl (62 kB)
Building wheel for ujson (pyproject.toml) ... done
DEPRECATED: Python 3.7 reached end of life
Successfully installed requests-2.31.0
`
	out := PackageManager(in)
	if len(out) >= len(in) {
		t.Errorf("expected progress noise stripped, got %d -> %d", len(in), len(out))
	}
	for _, must := range []string{"DEPRECATED", "Successfully installed"} {
		if !strings.Contains(out, must) {
			t.Errorf("important line dropped: %q\nfull output: %s", must, out)
		}
	}
}

// TestPackageManagerStripsCargoProgress targets cargo's Compiling/Fresh/
// Finished lines.
func TestPackageManagerStripsCargoProgress(t *testing.T) {
	in := `   Compiling syn v2.0.39
   Compiling proc-macro2 v1.0.69
   Compiling quote v1.0.33
warning: unused import: 'std::fmt'
   Finished release [optimized] target(s) in 12.34s
`
	out := PackageManager(in)
	if len(out) >= len(in) {
		t.Errorf("expected cargo-progress stripping, got %d -> %d", len(in), len(out))
	}
	if !strings.Contains(out, "warning: unused import") {
		t.Errorf("warning dropped: %q", out)
	}
}

func TestPackageManagerEmpty(t *testing.T) {
	if got := PackageManager(""); got != "" {
		t.Errorf("empty -> empty, got %q", got)
	}
}

func TestPackageManagerLosslessOnPlainText(t *testing.T) {
	in := "Just some text\nNo progress here\n"
	out := PackageManager(in)
	if len(out) > len(in) {
		t.Errorf("inflated: %d -> %d", len(in), len(out))
	}
}
