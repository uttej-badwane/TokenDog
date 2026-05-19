package filter

import (
	"strings"
	"testing"
)

var helmListSample = `NAME            NAMESPACE  REVISION  UPDATED                                  STATUS    CHART                APP VERSION
my-release      default    3         2024-05-01 12:00:00.123456789 +0000 UTC  deployed  nginx-1.2.3          1.25.0
other-release   staging    1         2024-04-01 08:00:00.000000000 +0000 UTC  deployed  myapp-0.9.0          0.9.0
`

func TestHelm_listNormalizesTable(t *testing.T) {
	out := Helm("list", helmListSample)
	// Should be shorter than the input (padding collapsed).
	if len(out) >= len(helmListSample) {
		t.Errorf("expected compression, got %d → %d bytes", len(helmListSample), len(out))
	}
	// Key values must survive.
	for _, want := range []string{"my-release", "nginx-1.2.3", "deployed", "other-release"} {
		if !strings.Contains(out, want) {
			t.Errorf("value %q missing after helm list compression", want)
		}
	}
}

func TestHelm_listShortenTimestamp(t *testing.T) {
	out := Helm("list", helmListSample)
	// Full nanosecond timestamps with timezone should be shortened.
	if strings.Contains(out, ".123456789") {
		t.Errorf("nanoseconds still present in timestamp: %q", out)
	}
	if strings.Contains(out, "+0000 UTC") {
		t.Errorf("timezone noise still present in timestamp: %q", out)
	}
}

func TestHelm_statusCollapesBlanks(t *testing.T) {
	input := "NAME: my-release\n\n\n\nSTATUS: deployed\n\nNOTES:\nHello\n"
	out := Helm("status", input)
	if strings.Contains(out, "\n\n\n") {
		t.Errorf("3+ consecutive blank lines not collapsed: %q", out)
	}
	if !strings.Contains(out, "deployed") || !strings.Contains(out, "Hello") {
		t.Error("content dropped")
	}
}

func TestHelm_diffPassthrough(t *testing.T) {
	input := "- old-value\n+ new-value\n"
	out := Helm("diff", input)
	if out != input {
		t.Errorf("diff output was modified: %q", out)
	}
}

func TestHelm_unknownSubcmdPassthrough(t *testing.T) {
	input := "some output\n"
	if Helm("install", input) != input {
		t.Error("unknown subcommand should pass through")
	}
}

func TestHelm_neverExpands(t *testing.T) {
	out := Helm("list", helmListSample)
	if len(out) > len(helmListSample) {
		t.Errorf("output longer than input")
	}
}
