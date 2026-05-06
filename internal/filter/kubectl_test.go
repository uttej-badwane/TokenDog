package filter

import (
	"strings"
	"testing"
)

func TestKubectlGetTableCompacts(t *testing.T) {
	in := `NAME                                 READY   STATUS    RESTARTS   AGE
api-server-7d4b9f9c-abc12            1/1     Running   0          5d
worker-pool-8c5d8f9d-xyz34           1/1     Running   2          3d
`
	out := Kubectl("get", in)
	if len(out) >= len(in) {
		t.Errorf("expected compaction, got %d -> %d", len(in), len(out))
	}
	for _, must := range []string{"api-server", "worker-pool", "Running"} {
		if !strings.Contains(out, must) {
			t.Errorf("lossless violation: %q missing", must)
		}
	}
}

func TestKubectlDescribeBlankCollapse(t *testing.T) {
	in := "Name: pod-x\n\n\nStatus: Running\n\n\nEvents:\n\n\n"
	out := Kubectl("describe", in)
	if strings.Contains(out, "\n\n\n") {
		t.Errorf("expected blank-line collapse, got %q", out)
	}
}

func TestKubectlPassthroughOnUnknownSubcmd(t *testing.T) {
	in := "anything goes here\n"
	out := Kubectl("apply", in)
	if out != in {
		t.Errorf("apply should passthrough, got %q", out)
	}
}

func TestKubectlEmpty(t *testing.T) {
	// Each subcommand may emit a single trailing newline from its line-
	// joiner. Acceptable; just verify no content is invented and length is
	// trivially small.
	for _, sub := range []string{"get", "describe", "top"} {
		got := Kubectl(sub, "")
		if len(got) > 1 {
			t.Errorf("Kubectl(%q, empty) = %q, want empty or single newline", sub, got)
		}
	}
}
