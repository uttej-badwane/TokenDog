package stash

import (
	"os"
	"testing"
)

func TestLogAndLoadRetrievals(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Nothing logged yet — not an error, just empty.
	rs, err := LoadRetrievals()
	if err != nil {
		t.Fatalf("LoadRetrievals on empty: %v", err)
	}
	if len(rs) != 0 {
		t.Fatalf("expected 0 retrievals, got %d", len(rs))
	}

	LogRetrieval("abc123", "kubectl describe pod x")
	LogRetrieval("def456", "journalctl -u nginx")
	LogRetrieval("abc123", "kubectl describe pod x")

	rs, err = LoadRetrievals()
	if err != nil {
		t.Fatalf("LoadRetrievals: %v", err)
	}
	if len(rs) != 3 {
		t.Fatalf("expected 3 retrievals, got %d", len(rs))
	}
	if rs[0].ID != "abc123" || rs[0].Command != "kubectl describe pod x" {
		t.Errorf("unexpected first record: %+v", rs[0])
	}
	if rs[0].At.IsZero() {
		t.Error("timestamp not recorded")
	}
}

func TestLoadRetrievalsSkipsMalformedLines(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path, err := retrievalsPath()
	if err != nil {
		t.Fatal(err)
	}
	// One valid line, one garbage line.
	content := `{"id":"x","command":"cat big.json","at":"2026-06-01T00:00:00Z"}` + "\n" +
		"this is not json\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	rs, err := LoadRetrievals()
	if err != nil {
		t.Fatalf("LoadRetrievals: %v", err)
	}
	if len(rs) != 1 {
		t.Fatalf("expected malformed line skipped (1 valid), got %d", len(rs))
	}
}
