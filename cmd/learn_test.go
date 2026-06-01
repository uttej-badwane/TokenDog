package cmd

import "testing"

func TestReversibleCommand(t *testing.T) {
	cases := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"proxy: journalctl -u nginx (reversible)", "journalctl -u nginx", true},
		{"proxy: kubectl describe pod x (reversible)", "kubectl describe pod x", true},
		{"proxy: git status", "", false},         // proxy record, but not reversible
		{"proxy: git status (dedup)", "", false}, // different proxy transform
		{"git status", "", false},                // not a proxy record at all
		{"proxy:  (reversible)", "", true},       // empty command still parses
	}
	for _, c := range cases {
		got, ok := reversibleCommand(c.in)
		if ok != c.wantOK || (ok && got != c.want) {
			t.Errorf("reversibleCommand(%q) = (%q,%v), want (%q,%v)", c.in, got, ok, c.want, c.wantOK)
		}
	}
}

func TestLearnBinary(t *testing.T) {
	cases := map[string]string{
		"kubectl describe pod x":             "kubectl",
		"/usr/local/bin/journalctl -u nginx": "journalctl",
		"AWS_PROFILE=prod aws s3 ls":         "aws",
		"FOO=bar BAZ=qux psql -c 'select 1'": "psql",
		"cat config.yaml":                    "cat",
		"":                                   "(unknown)",
	}
	for in, want := range cases {
		if got := learnBinary(in); got != want {
			t.Errorf("learnBinary(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsEnvName(t *testing.T) {
	yes := []string{"AWS_PROFILE", "FOO", "a1", "_x"}
	no := []string{"", "1abc", "foo-bar", "a.b", "x=y"}
	for _, s := range yes {
		if !isEnvName(s) {
			t.Errorf("isEnvName(%q) = false, want true", s)
		}
	}
	for _, s := range no {
		if isEnvName(s) {
			t.Errorf("isEnvName(%q) = true, want false", s)
		}
	}
}
