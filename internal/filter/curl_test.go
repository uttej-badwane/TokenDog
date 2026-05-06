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
