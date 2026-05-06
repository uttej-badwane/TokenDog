package filter

import (
	"strings"
	"testing"
)

func TestJQCompactsObject(t *testing.T) {
	in := `{
  "name": "value",
  "nested": {
    "key": 42
  }
}
`
	out := JQ(in)
	if len(out) >= len(in) {
		t.Errorf("expected compaction, got %d -> %d", len(in), len(out))
	}
	for _, must := range []string{"name", "value", "nested", "key", "42"} {
		if !strings.Contains(out, must) {
			t.Errorf("lossless violation: %q missing", must)
		}
	}
}

func TestJQCompactsArray(t *testing.T) {
	in := `[
  "a",
  "b",
  "c"
]
`
	out := JQ(in)
	if !strings.Contains(out, `["a","b","c"]`) {
		t.Errorf("expected array compaction, got %q", out)
	}
}

func TestJQHandlesMultipleDocs(t *testing.T) {
	// jq's default output is one JSON value per line. Compact each.
	in := `{"a": 1}
{"b": 2}
`
	out := JQ(in)
	if !strings.Contains(out, `{"a":1}`) || !strings.Contains(out, `{"b":2}`) {
		t.Errorf("multi-doc compaction broken: %q", out)
	}
}

func TestJQLargeArraySummarizes(t *testing.T) {
	// >50 items: compactJSONValue prepends "[N items]" but keeps the data.
	var b strings.Builder
	b.WriteString("[")
	for i := 0; i < 60; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("1")
	}
	b.WriteString("]")
	out := JQ(b.String())
	if !strings.Contains(out, "[60 items]") {
		t.Errorf("expected [60 items] prefix on large array, got prefix: %q", out[:30])
	}
}

func TestJQPassthroughOnInvalid(t *testing.T) {
	in := "not json at all\nplain\n"
	out := JQ(in)
	// compactJSONValue returns input unchanged on parse failure — so each
	// line gets re-emitted as its own "doc". Result must not inflate.
	if len(out) > len(in)+5 { // tolerate trailing newline
		t.Errorf("non-json inflated: %d -> %d", len(in), len(out))
	}
}
