package cmd

import "testing"

// TestExtractSubcommandValueFlags is the regression test for v0.4.4's
// "subcmd misidentified as flag value" bug. Before the fix,
// `git -C /path status` returned subcmd="/path" because the loop just
// took the first non-flag arg.
func TestExtractSubcommandValueFlags(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		valueFlags map[string]bool
		want       string
	}{
		{"plain", []string{"status"}, gitValueFlags, "status"},
		{"git -C path subcmd", []string{"-C", "/some/path", "status"}, gitValueFlags, "status"},
		{"git --git-dir=foo subcmd (=value form)", []string{"--git-dir=/path", "status"}, gitValueFlags, "status"},
		{"git --git-dir foo subcmd (split form)", []string{"--git-dir", "/path", "status"}, gitValueFlags, "status"},
		{"gh --repo owner/name subcmd", []string{"--repo", "uttej-badwane/TokenDog", "pr", "list"}, ghValueFlags, "pr"},
		{"gh -R owner/name subcmd", []string{"-R", "uttej-badwane/TokenDog", "pr", "list"}, ghValueFlags, "pr"},
		{"kubectl --context foo get pods", []string{"--context", "kind-c1", "get", "pods"}, kubectlValueFlags, "get"},
		{"kubectl -n ns get pods", []string{"-n", "kube-system", "get", "pods"}, kubectlValueFlags, "get"},
		{"docker -H host ps", []string{"-H", "tcp://1.2.3.4:2375", "ps"}, dockerValueFlags, "ps"},
		{"empty args", []string{}, gitValueFlags, ""},
		{"all flags, no subcmd", []string{"--version"}, gitValueFlags, ""},
		{"mix of flags before subcmd", []string{"-C", "/x", "--git-dir=/y", "log"}, gitValueFlags, "log"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractSubcommand(tc.args, tc.valueFlags); got != tc.want {
				t.Errorf("extractSubcommand(%v) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}
