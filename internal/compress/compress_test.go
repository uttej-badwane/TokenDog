package compress

import (
	"strings"
	"testing"
)

func TestCompressString_fillers(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"just run the tests", "run tests"},
		{"basically this is fine", "this is fine"},
		{"you should always run tests before pushing", "always run tests before pushing"},
		// "always" is a semantic qualifier, not a filler — keep it.
		{"I'll help you fix this", "help you fix this"},
		{"Let me look at the code", "look at code"},
		{"in order to build the binary", "to build binary"},
		{"make sure to restart the server", "ensure restart server"},
		{"Sure, I can help with that", "I can help with that"},
		{"Of course, this is easy", "this is easy"},
		{"It is important to note that config changes require restart", "config changes require restart"},
	}
	for _, tc := range cases {
		got := CompressString(tc.in)
		if got != tc.want {
			t.Errorf("CompressString(%q)\n  got  %q\n  want %q", tc.in, got, tc.want)
		}
	}
}

func TestCompressString_preservesCode(t *testing.T) {
	code := "```go\nfunc main() { fmt.Println(\"hello\") }\n```"
	in := "Just run " + code + " to build the binary."
	out := CompressString(in)
	if !strings.Contains(out, "func main()") {
		t.Errorf("code block was modified: %q", out)
	}
	if strings.Contains(out, "Just") {
		t.Errorf("filler 'Just' not stripped from prose: %q", out)
	}
}

func TestCompressString_preservesURLs(t *testing.T) {
	in := "See the docs at https://docs.anthropic.com/en/api/getting-started for more info."
	out := CompressString(in)
	if !strings.Contains(out, "https://docs.anthropic.com/en/api/getting-started") {
		t.Errorf("URL was modified: %q", out)
	}
}

func TestCompressString_preservesInlineCode(t *testing.T) {
	in := "Run `td setup` to configure the proxy."
	out := CompressString(in)
	if !strings.Contains(out, "`td setup`") {
		t.Errorf("inline code was modified: %q", out)
	}
}

func TestCompressString_neverExpands(t *testing.T) {
	cases := []string{
		"short",
		"already terse: run tests.",
		"```\ncode only\n```",
		"https://example.com",
		"",
	}
	for _, in := range cases {
		out := CompressString(in)
		if len(out) > len(in) {
			t.Errorf("output longer than input for %q (got %q)", in, out)
		}
	}
}

func TestCompressFile_unchanged(t *testing.T) {
	// Already-compressed content should come back unchanged=false.
	content := "Run tests before push. Fix bugs. No filler here."
	_, changed := CompressFile(content)
	if changed {
		t.Error("expected no change on already-terse content")
	}
}

func TestCompressFile_markdown(t *testing.T) {
	content := `# Setup

You should always make sure to run the tests before pushing any changes.
In order to build the binary, just run the make command.

` + "```" + `bash
make build
` + "```" + `

Additionally, please note that the proxy requires TouchID on macOS.`

	compressed, changed := CompressFile(content)
	if !changed {
		t.Fatal("expected content to be compressed")
	}
	// Code block must be untouched
	if !strings.Contains(compressed, "make build") {
		t.Error("code block was modified")
	}
	// Heading must survive
	if !strings.Contains(compressed, "# Setup") {
		t.Error("heading was removed")
	}
	// Fillers should be gone
	for _, filler := range []string{"just", "additionally", "please note that", "always make sure to"} {
		if strings.Contains(strings.ToLower(compressed), filler) {
			t.Errorf("filler %q still present in: %q", filler, compressed)
		}
	}
	if len(compressed) >= len(content) {
		t.Errorf("compressed (%d) not smaller than original (%d)", len(compressed), len(content))
	}
}
