package tokenizer

import "testing"

func TestEstimateFromBytes(t *testing.T) {
	cases := []struct {
		in, want int
	}{
		{0, 0},
		{1, 1},
		{4, 1},
		{5, 2},
		{100, 25},
	}
	for _, tc := range cases {
		if got := estimateFromBytes(tc.in); got != tc.want {
			t.Errorf("estimateFromBytes(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// TestCountWorks verifies Count returns a non-negative number for typical
// inputs. We don't assert exact counts because the value depends on whether
// tiktoken's vocab loaded; either way the function must not panic.
func TestCountWorks(t *testing.T) {
	for _, s := range []string{"", "hello", "hello world", "{\"key\": \"value\"}"} {
		n := Count(s)
		if n < 0 {
			t.Errorf("Count(%q) = %d, expected >= 0", s, n)
		}
	}
}
