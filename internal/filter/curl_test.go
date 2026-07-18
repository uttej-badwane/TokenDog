package filter

import (
	"strings"
	"testing"
)

func TestCurlJSONResponse(t *testing.T) {
	in := `{
  "id": 12345,
  "name": "test",
  "items": [
    {"sub": 1},
    {"sub": 2}
  ]
}
`
	out := Curl(in)
	if len(out) >= len(in) {
		t.Errorf("JSON response should compact: %d -> %d", len(in), len(out))
	}
	if !strings.Contains(out, "12345") || !strings.Contains(out, "test") {
		t.Errorf("lossless violation: %q", out)
	}
}

func TestCurlPassthroughOnHTML(t *testing.T) {
	// HTML response: filter must not corrupt it. May trim trailing whitespace
	// (lossless contract allows that) but must preserve all content bytes.
	in := "<html><body>hello</body></html>\n"
	out := Curl(in)
	if !strings.Contains(out, "<html><body>hello</body></html>") {
		t.Errorf("HTML body lost: got %q", out)
	}
	if len(out) > len(in) {
		t.Errorf("inflated: %d -> %d", len(in), len(out))
	}
}

func TestCurlEmptyInput(t *testing.T) {
	if got := Curl(""); got != "" {
		t.Errorf("empty input should return empty, got %q", got)
	}
}

func TestCurlStripsScriptStyleComment(t *testing.T) {
	in := `<!doctype html><html><head>` +
		`<style>.x{color:red}</style>` +
		`<script>var a = 1; document.write("junk");</script>` +
		`</head><body><!-- tracking pixel --><p>Real content here.</p></body></html>`
	out := Curl(in)
	if len(out) >= len(in) {
		t.Errorf("script/style/comment should shrink HTML: %d -> %d", len(in), len(out))
	}
	// Visible content preserved; invisible noise gone.
	if !strings.Contains(out, "Real content here.") {
		t.Errorf("visible content lost: %q", out)
	}
	for _, gone := range []string{"color:red", "document.write", "tracking pixel"} {
		if strings.Contains(out, gone) {
			t.Errorf("noise %q survived: %q", gone, out)
		}
	}
	// Curl keeps markup (lossless path) — the tag reduction is HTMLToText's job.
	if !strings.Contains(out, "<p>") {
		t.Errorf("Curl should keep visible markup, dropped <p>: %q", out)
	}
}

func TestHTMLToTextExtractsVisibleText(t *testing.T) {
	in := `<html><head><title>T</title><script>x()</script></head>` +
		`<body><h1>Title</h1><p>First&nbsp;para with <a href="/x">a link</a>.</p>` +
		`<ul><li>one</li><li>two</li></ul></body></html>`
	out := HTMLToText(in)
	if strings.Contains(out, "<") {
		t.Errorf("tags survived: %q", out)
	}
	if strings.Contains(out, "x()") {
		t.Errorf("script body survived: %q", out)
	}
	for _, want := range []string{"Title", "First", "para with", "a link", "one", "two"} {
		if !strings.Contains(out, want) {
			t.Errorf("visible text %q missing: %q", want, out)
		}
	}
	// &nbsp; must decode, not linger as an entity.
	if strings.Contains(out, "&nbsp;") {
		t.Errorf("entity not decoded: %q", out)
	}
	if len(out) >= len(in) {
		t.Errorf("HTMLToText should shrink: %d -> %d", len(in), len(out))
	}
}

func TestHTMLToTextAdversarialNoPanic(t *testing.T) {
	// Unclosed tags, stray angle brackets, truncated script — must not panic
	// and must not inflate.
	for _, in := range []string{
		"<script>never closed",
		"<html><body><p>a < b and c > d",
		"<<<>>><div",
		"<!-- unterminated comment <p>hi",
	} {
		out := HTMLToText(in)
		if len(out) > len(in) {
			t.Errorf("inflated on %q: %d -> %d", in, len(in), len(out))
		}
	}
}

func TestLooksLikeHTMLConservative(t *testing.T) {
	yes := []string{"<!doctype html><html>", "<html lang=\"en\">", "  <div>x</div>", "<meta charset>"}
	no := []string{"{\"a\":1}", "plain text", "<?xml version=\"1.0\"?><rss>", "a < b < c"}
	for _, s := range yes {
		if !LooksLikeHTML(s) {
			t.Errorf("expected HTML: %q", s)
		}
	}
	for _, s := range no {
		if LooksLikeHTML(s) {
			t.Errorf("expected non-HTML: %q", s)
		}
	}
}
