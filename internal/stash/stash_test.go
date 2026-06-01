package stash

import (
	"strings"
	"testing"
)

// withTempHome redirects HOME so stash writes land in a per-test temp dir and
// enables the feature (it is opt-in via TD_REVERSIBLE).
func withTempHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TD_REVERSIBLE", "1")
}

func TestEnabledOptIn(t *testing.T) {
	t.Setenv("TD_REVERSIBLE", "")
	if Enabled() {
		t.Fatal("reversible compression should be off by default")
	}
	for _, v := range []string{"1", "true", "YES", "on"} {
		t.Setenv("TD_REVERSIBLE", v)
		if !Enabled() {
			t.Errorf("TD_REVERSIBLE=%q should enable", v)
		}
	}
}

func TestPutGetRoundTrip(t *testing.T) {
	withTempHome(t)
	content := "line1\nline2\nsecret middle\nline4\n"
	id, err := Put("git log", content)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if id == "" {
		t.Fatal("empty id")
	}
	rec, ok := Get(id)
	if !ok {
		t.Fatal("Get miss after Put")
	}
	if rec.Content != content {
		t.Errorf("content round-trip mismatch:\ngot  %q\nwant %q", rec.Content, content)
	}
	if rec.OrigBytes != len(content) {
		t.Errorf("OrigBytes = %d, want %d", rec.OrigBytes, len(content))
	}
	if rec.Command != "git log" {
		t.Errorf("Command = %q, want %q", rec.Command, "git log")
	}
}

func TestPutIsContentAddressed(t *testing.T) {
	withTempHome(t)
	id1, _ := Put("cmd a", "same payload")
	id2, _ := Put("cmd b", "same payload")
	if id1 != id2 {
		t.Errorf("identical content should dedupe to one id: %s vs %s", id1, id2)
	}
	id3, _ := Put("cmd a", "different payload")
	if id3 == id1 {
		t.Errorf("different content must not collide: %s", id3)
	}
}

func TestGetMissReturnsFalse(t *testing.T) {
	withTempHome(t)
	if _, ok := Get("deadbeefdead"); ok {
		t.Error("Get on unknown id should miss")
	}
}

func TestPreviewElidesMiddleAndKeepsEnds(t *testing.T) {
	id := "abc123def456"
	var lines []string
	for i := 0; i < 100; i++ {
		lines = append(lines, "L"+string(rune('0'+i%10)))
	}
	content := strings.Join(lines, "\n")
	out := Preview(id, content, 3, 2)

	if !strings.Contains(out, "td:STASHED id="+id) {
		t.Error("preview missing stash marker with id")
	}
	if !strings.Contains(out, "td_retrieve") {
		t.Error("preview marker should name the retrieval tool")
	}
	if len(out) >= len(content) {
		t.Errorf("preview not smaller: %d >= %d", len(out), len(content))
	}
	// First and last lines must survive verbatim.
	if !strings.HasPrefix(out, lines[0]) {
		t.Error("preview dropped the head")
	}
	if !strings.HasSuffix(out, lines[len(lines)-1]) {
		t.Error("preview dropped the tail")
	}
}

func TestPreviewShortContentUnchanged(t *testing.T) {
	content := "a\nb\nc"
	if out := Preview("id", content, 20, 5); out != content {
		t.Errorf("short content should pass through unchanged, got %q", out)
	}
}

func TestListAndPurge(t *testing.T) {
	withTempHome(t)
	Put("cmd1", "payload one is here")
	Put("cmd2", "payload two is here")

	recs, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("List returned %d, want 2", len(recs))
	}

	n, err := Purge()
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if n != 2 {
		t.Errorf("Purge removed %d, want 2", n)
	}
	recs, _ = List()
	if len(recs) != 0 {
		t.Errorf("List after purge returned %d, want 0", len(recs))
	}
}
