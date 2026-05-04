package analytics

import (
	"strings"
	"testing"
)

// TestProgressBarNegativePct is the regression test for v0.4.3's
// "strings: negative Repeat count" panic. When a record's filter produced
// more bytes than the raw input (find on small inputs, etc.) SavedPct
// went negative — Repeat panics on negative counts.
func TestProgressBarNegativePct(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("progressBar panicked on negative pct: %v", r)
		}
	}()
	bar := progressBar(-12.5, 10)
	if !strings.Contains(bar, "░") {
		t.Errorf("expected empty bar for negative pct, got: %q", bar)
	}
}

func TestProgressBarOverflow(t *testing.T) {
	bar := progressBar(150, 10)
	// Should clamp to fully filled.
	if strings.Count(bar, "█") != 10 {
		t.Errorf("expected fully filled bar for >100%%, got: %q (filled=%d)", bar, strings.Count(bar, "█"))
	}
}

func TestProgressBarBoundaries(t *testing.T) {
	cases := []struct {
		pct      float64
		width    int
		filledOK func(int) bool
	}{
		{0, 10, func(n int) bool { return n == 0 }},
		{50, 10, func(n int) bool { return n == 5 }},
		{100, 10, func(n int) bool { return n == 10 }},
	}
	for _, tc := range cases {
		bar := progressBar(tc.pct, tc.width)
		filled := strings.Count(bar, "█")
		if !tc.filledOK(filled) {
			t.Errorf("pct=%v width=%d: filled=%d, bar=%q", tc.pct, tc.width, filled, bar)
		}
	}
}

func TestSavedPctZeroRaw(t *testing.T) {
	r := Record{RawBytes: 0, FilteredBytes: 0}
	if got := r.SavedPct(); got != 0 {
		t.Errorf("SavedPct() with raw=0 = %v, want 0 (no NaN, no panic)", got)
	}
}

func TestSavedPctNegative(t *testing.T) {
	// Records where filter inflated output (rare but allowed) should
	// produce a negative SavedPct without breaking anything.
	r := Record{RawBytes: 100, FilteredBytes: 110}
	got := r.SavedPct()
	if got >= 0 {
		t.Errorf("SavedPct() with filt>raw should be negative, got %v", got)
	}
}

func TestNormalizeName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"td git status", "git status"},
		{"tokendog git log", "git log"},
		{"git status", "git status"},
		{"td ls", "ls"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := normalizeName(tc.in); got != tc.want {
			t.Errorf("normalizeName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
