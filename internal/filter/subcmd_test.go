package filter

import "testing"

// TestExtractSubcmdValueFlags is the regression test for the
// `git -C path status` → "status" (not "path") classification. Without
// value-flag awareness, ExtractSubcmd returns the first non-dash arg
// which would be the path passed to -C.
func TestExtractSubcmdValueFlags(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"git status", []string{"status"}, "status"},
		{"git -C path status", []string{"-C", "/tmp", "status"}, "status"},
		{"git --git-dir=/x status", []string{"--git-dir=/x", "status"}, "status"},
		{"git -c key=val log", []string{"-c", "user.email=foo", "log"}, "log"},
		{"only flags", []string{"-C", "/tmp"}, ""},
		{"empty", []string{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractSubcmd(tc.args, gitValueFlags)
			if got != tc.want {
				t.Errorf("ExtractSubcmd(%v) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

func TestSubcmdForBinary(t *testing.T) {
	cases := []struct {
		bin  string
		args []string
		want string
	}{
		{"git", []string{"-C", "/tmp", "status"}, "status"},
		{"gh", []string{"--repo", "x/y", "pr", "view", "123"}, "pr"},
		{"kubectl", []string{"-n", "kube-system", "get", "pods"}, "get"},
		{"docker", []string{"-H", "tcp://x", "ps", "-a"}, "ps"},
		{"go", []string{"test", "./..."}, "test"},
		{"go", []string{"-v", "build", "./..."}, "build"},
		// Tools without a subcommand concept return "".
		{"find", []string{".", "-name", "*.go"}, ""},
		{"jq", []string{".items[]"}, ""},
		// Unregistered binary returns "".
		{"grep", []string{"-r", "foo"}, ""},
	}
	for _, tc := range cases {
		got := SubcmdFor(tc.bin, tc.args)
		if got != tc.want {
			t.Errorf("SubcmdFor(%q, %v) = %q, want %q", tc.bin, tc.args, got, tc.want)
		}
	}
}

func TestRegisteredAreUnique(t *testing.T) {
	// Sanity: every Supported binary should have a registered filter and
	// vice versa. If you add a Supported binary without registering, the
	// hook will rewrite to td but `td <bin>` will fall through to passthru.
	seen := map[string]bool{}
	for _, b := range Registered() {
		if seen[b] {
			t.Errorf("duplicate registration for %q", b)
		}
		seen[b] = true
	}
}
