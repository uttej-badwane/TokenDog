package filter

import "testing"

func TestGuard(t *testing.T) {
	cases := []struct {
		name, raw, filtered, want string
	}{
		{"smaller", "hello world", "hi", "hi"},
		{"equal", "hello", "world", "hello"},
		{"larger", "hi", "hi there", "hi"},
		{"empty raw", "", "", ""},
		{"empty filtered", "hello", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Guard(tc.raw, tc.filtered); got != tc.want {
				t.Errorf("Guard(%q, %q) = %q, want %q", tc.raw, tc.filtered, got, tc.want)
			}
		})
	}
}
