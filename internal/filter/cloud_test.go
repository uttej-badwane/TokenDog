package filter

import (
	"strings"
	"testing"
)

// TestCloudJSONCompaction is the headline lossless test: a real-shape
// `aws ec2 describe-instances` snippet compacts via Marshal-without-indent
// while preserving every value.
func TestCloudJSONCompaction(t *testing.T) {
	in := `{
  "Reservations": [
    {
      "Instances": [
        {
          "InstanceId": "i-0123456789abcdef0",
          "InstanceType": "m5.large",
          "State": { "Name": "running", "Code": 16 }
        }
      ]
    }
  ]
}
`
	out := Cloud(in)
	if len(out) >= len(in) {
		t.Errorf("expected compaction, got %d -> %d bytes", len(in), len(out))
	}
	for _, must := range []string{"i-0123456789abcdef0", "m5.large", "running"} {
		if !strings.Contains(out, must) {
			t.Errorf("lossless violation: %q missing from output\n%s", must, out)
		}
	}
}

func TestCloudPassthroughOnNonJSON(t *testing.T) {
	in := "Bucket: my-bucket\nObjects: 42\n"
	out := Cloud(in)
	if out != in {
		t.Errorf("plain text should pass through, got %q", out)
	}
}

func TestCloudHandlesMalformedJSON(t *testing.T) {
	in := `{ "key": "value", "broken":  `
	out := Cloud(in)
	// Lossless contract: never produce more bytes; either cleanly compact or
	// pass through. compactJSONValue returns input unchanged on parse error.
	if len(out) > len(in) {
		t.Errorf("malformed JSON inflated: %d -> %d", len(in), len(out))
	}
}

func TestCloudArrayCompaction(t *testing.T) {
	in := `[
  {"a": 1},
  {"a": 2}
]
`
	out := Cloud(in)
	if !strings.Contains(out, `[{"a":1},{"a":2}]`) {
		t.Errorf("array not compacted: %q", out)
	}
}

func TestCloudYAMLBlankLineCollapse(t *testing.T) {
	// gcloud config get-value etc. emits YAML with double-blank-line padding.
	in := "key1: value1\n\n\nkey2: value2\n\n\nkey3: value3\n"
	out := Cloud(in)
	if strings.Contains(out, "\n\n\n") {
		t.Errorf("expected blank-line collapse, got: %q", out)
	}
	for _, must := range []string{"key1", "key2", "key3", "value1", "value2", "value3"} {
		if !strings.Contains(out, must) {
			t.Errorf("lossless violation: %q missing", must)
		}
	}
}
