package filter

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGenericCompactsIndentedJSONObject(t *testing.T) {
	pretty := `{
  "name": "tokendog",
  "nested": {
    "a": 1,
    "b": [1, 2, 3]
  },
  "flag": true
}`
	out, ok := Generic(pretty)
	if !ok {
		t.Fatal("expected generic to compact indented JSON")
	}
	if len(out) >= len(pretty) {
		t.Errorf("expected smaller output, got %d >= %d", len(out), len(pretty))
	}
	// Output must still be valid JSON encoding the same value.
	var a, b any
	if err := json.Unmarshal([]byte(pretty), &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(out), &b); err != nil {
		t.Fatalf("compacted output is not valid JSON: %v", err)
	}
	if !jsonEqual(a, b) {
		t.Error("compacted JSON is not semantically equal to the original")
	}
	if strings.Contains(out, "\n  ") {
		t.Error("compacted output still contains indentation")
	}
}

func TestGenericCompactsJSONArray(t *testing.T) {
	pretty := "[\n  {\n    \"id\": 1\n  },\n  {\n    \"id\": 2\n  }\n]"
	out, ok := Generic(pretty)
	if !ok {
		t.Fatal("expected array compaction")
	}
	if len(out) >= len(pretty) {
		t.Errorf("no reduction: %d >= %d", len(out), len(pretty))
	}
}

func TestGenericSkipsNonJSON(t *testing.T) {
	for _, in := range []string{
		"On branch main\n\tmodified: foo.go\n",
		"",
		"   ",
		"plain log line without structure",
		`{"unterminated": `,          // invalid JSON
		`{"a":1}` + "\n" + `{"b":2}`, // two documents — a JSON stream, not one value
		`{"a":1} trailing junk`,      // trailing non-whitespace
		"true",                       // bare scalar
		`{"a":1}`,                    // already compact — no win
	} {
		if out, ok := Generic(in); ok {
			t.Errorf("Generic should not have fired on %q (got %q)", in, out)
		}
	}
}

func TestGenericSkipsWhenAlreadyCompact(t *testing.T) {
	compact := `{"a":1,"b":2,"c":[1,2,3]}`
	if _, ok := Generic(compact); ok {
		t.Error("already-compact JSON should yield no win")
	}
}

// jsonEqual compares two decoded JSON values for deep equality.
func jsonEqual(a, b any) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ab) == string(bb)
}
