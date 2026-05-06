package replay

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeDispatch echoes "OK\n" for any git command (handled), passes through
// echo (unhandled). Tests that the walker correctly matches tool_use to
// tool_result regardless of order or what's in between.
func fakeDispatch(cmd, raw string) (string, DispatchInfo) {
	parts := strings.Fields(cmd)
	if len(parts) > 0 && parts[0] == "git" {
		sub := ""
		for _, a := range parts[1:] {
			if !strings.HasPrefix(a, "-") {
				sub = a
				break
			}
		}
		return "OK\n", DispatchInfo{Handled: true, Binary: "git", Subcmd: sub}
	}
	return raw, DispatchInfo{Handled: false}
}

func writeTranscript(t *testing.T, lines []string) string {
	t.Helper()
	dir := t.TempDir()
	f := filepath.Join(dir, "session.jsonl")
	if err := os.WriteFile(f, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return dir
}

func TestWalkMatchesUseToResult(t *testing.T) {
	lines := []string{
		// assistant msg with a Bash tool_use for `git log` (handled)
		`{"type":"assistant","sessionId":"s1","timestamp":"2026-05-05T10:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"git log -3"}}]}}`,
		// user msg with the matching tool_result
		`{"type":"user","sessionId":"s1","timestamp":"2026-05-05T10:00:01Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"commit abc\nMessage\n"}]}}`,
		// another tool_use for `echo` (NOT handled — fakeDispatch rejects)
		`{"type":"assistant","sessionId":"s1","timestamp":"2026-05-05T10:00:02Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_2","name":"Bash","input":{"command":"echo hi"}}]}}`,
		`{"type":"user","sessionId":"s1","timestamp":"2026-05-05T10:00:03Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_2","content":"hi\n"}]}}`,
	}
	dir := writeTranscript(t, lines)

	r, err := Walk(dir, fakeDispatch, Options{})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if r.SessionsScanned != 1 {
		t.Errorf("SessionsScanned = %d, want 1", r.SessionsScanned)
	}
	if r.BashCallsSeen != 2 {
		t.Errorf("BashCallsSeen = %d, want 2", r.BashCallsSeen)
	}
	if r.BashCallsHandled != 1 {
		t.Errorf("BashCallsHandled = %d, want 1 (git only)", r.BashCallsHandled)
	}
	if r.UnhandledTopN["echo"] != 1 {
		t.Errorf("UnhandledTopN[echo] = %d, want 1", r.UnhandledTopN["echo"])
	}
	cs, ok := r.PerCommand["git log"]
	if !ok {
		t.Fatal("expected PerCommand[git log] to exist")
	}
	if cs.Calls != 1 {
		t.Errorf("git log Calls = %d, want 1", cs.Calls)
	}
	if r.RawBytes <= len("OK\n") {
		t.Errorf("RawBytes = %d, expected > filter output", r.RawBytes)
	}
}

func TestWalkArrayContent(t *testing.T) {
	// tool_result content can be an array of {type:"text", text:"..."}.
	lines := []string{
		`{"type":"assistant","sessionId":"s2","timestamp":"2026-05-05T10:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"u1","name":"Bash","input":{"command":"git status"}}]}}`,
		`{"type":"user","sessionId":"s2","timestamp":"2026-05-05T10:00:01Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"u1","content":[{"type":"text","text":"On branch main\n"}]}]}}`,
	}
	dir := writeTranscript(t, lines)
	r, err := Walk(dir, fakeDispatch, Options{})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if r.BashCallsHandled != 1 {
		t.Errorf("BashCallsHandled = %d, want 1 (array content should still parse)", r.BashCallsHandled)
	}
}

func TestWalkSkipsOrphans(t *testing.T) {
	// tool_result with no matching tool_use — should be silently dropped.
	lines := []string{
		`{"type":"user","sessionId":"s3","timestamp":"2026-05-05T10:00:00Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"missing","content":"orphan"}]}}`,
	}
	dir := writeTranscript(t, lines)
	r, err := Walk(dir, fakeDispatch, Options{})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if r.BashCallsSeen != 0 {
		t.Errorf("BashCallsSeen = %d, want 0", r.BashCallsSeen)
	}
}

func TestFirstWord(t *testing.T) {
	cases := []struct{ in, want string }{
		{"git status", "git"},
		{"AWS_PROFILE=p aws ec2", "aws"},
		{"/usr/local/bin/gh pr view", "gh"},
		{"FOO=bar BAZ=qux echo hi", "echo"},
		{"", ""},
		{"FOO=bar", ""},
	}
	for _, tc := range cases {
		if got := firstWord(tc.in); got != tc.want {
			t.Errorf("firstWord(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDispatchInfoKey(t *testing.T) {
	if got := (DispatchInfo{Binary: "git", Subcmd: "status"}).Key(); got != "git status" {
		t.Errorf("Key() = %q, want %q", got, "git status")
	}
	if got := (DispatchInfo{Binary: "make"}).Key(); got != "make" {
		t.Errorf("Key() bare-binary = %q, want %q", got, "make")
	}
}
