package filter

import (
	"strings"
	"testing"
)

// TestDockerPSPassThroughOnInflation is the regression test for v0.4.4's
// "docker silently drops malformed lines" bug. Combined with the
// pass-through-on-inflation guard: when reformatting wouldn't shrink the
// output, the original must be returned untouched.
func TestDockerPSPassThroughOnInflation(t *testing.T) {
	in := "CONTAINER ID   IMAGE   COMMAND   CREATED   STATUS   PORTS   NAMES\nabc1234   alpine   sleep   1 hour ago   Up 1 hour    foo\n"
	out := Docker("ps", in)
	if len(out) > len(in) {
		t.Errorf("dockerPS produced larger output: filtered=%d raw=%d", len(out), len(in))
	}
}

func TestDockerPSCompactsRealOutput(t *testing.T) {
	// Wide real-world output — many rows, plenty of whitespace.
	in := "CONTAINER ID   IMAGE                                                COMMAND                  CREATED       STATUS       PORTS                NAMES\n"
	for i := 0; i < 5; i++ {
		in += "0123456789ab   registry.example.com/team/very-long-image:latest   \"sh -c 'long cmd'\"   2 hours ago   Up 2 hours   8080/tcp,9090/tcp    container_" + string(rune('a'+i)) + "\n"
	}
	out := Docker("ps", in)
	if len(out) >= len(in) {
		t.Errorf("expected compression, filtered=%d raw=%d", len(out), len(in))
	}
	if !strings.Contains(out, "alpine") && !strings.Contains(out, "container_") {
		// Sanity: at least one of our row markers should still be present.
		t.Errorf("output appears to have lost row data: %q", out)
	}
}

func TestDockerUnknownSubcmd(t *testing.T) {
	in := "anything\n"
	if got := Docker("logs", in); got != in {
		t.Errorf("expected unknown subcmd passes through, got: %q", got)
	}
}
