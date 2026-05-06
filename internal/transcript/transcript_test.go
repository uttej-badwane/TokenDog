package transcript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTranscript dumps lines to a temp file and returns the path.
func writeTranscript(t *testing.T, lines []string) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(f, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return f
}

func TestReadBasic(t *testing.T) {
	lines := []string{
		`{"type":"permission-mode","sessionId":"abc","permissionMode":"default"}`,
		`{"type":"assistant","sessionId":"abc","timestamp":"2026-05-05T10:00:00Z","isSidechain":false,"message":{"stop_reason":"end_turn","usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":1000,"cache_read_input_tokens":500}}}`,
		`{"type":"user","sessionId":"abc","timestamp":"2026-05-05T10:01:00Z","isSidechain":false,"message":{"stop_reason":null,"usage":{"input_tokens":10,"output_tokens":0}}}`,
	}
	totals, err := Read(writeTranscript(t, lines))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if totals.SessionID != "abc" {
		t.Errorf("SessionID = %q, want %q", totals.SessionID, "abc")
	}
	if totals.InputTokens != 110 {
		t.Errorf("InputTokens = %d, want 110", totals.InputTokens)
	}
	if totals.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d, want 50", totals.OutputTokens)
	}
	if totals.CacheCreationTokens != 1000 || totals.CacheReadTokens != 500 {
		t.Errorf("cache totals: creation=%d read=%d, want 1000/500", totals.CacheCreationTokens, totals.CacheReadTokens)
	}
	if totals.NumAPICalls != 2 {
		t.Errorf("NumAPICalls = %d, want 2", totals.NumAPICalls)
	}
}

// TestStreamingDedup: a streaming response writes two rows for the same API
// call — an intermediate (stop_reason: null) and a final (stop_reason:
// end_turn). The intermediate row's usage should be dropped to avoid double-
// counting.
func TestStreamingDedup(t *testing.T) {
	lines := []string{
		// Intermediate streaming row — should NOT be counted.
		`{"type":"assistant","sessionId":"x","timestamp":"2026-05-05T10:00:00Z","message":{"stop_reason":null,"usage":{"input_tokens":50,"output_tokens":10}}}`,
		// Finalized row for the same call — should be counted.
		`{"type":"assistant","sessionId":"x","timestamp":"2026-05-05T10:00:01Z","message":{"stop_reason":"end_turn","usage":{"input_tokens":100,"output_tokens":25}}}`,
	}
	totals, _ := Read(writeTranscript(t, lines))
	if totals.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100 (intermediate row should be dropped)", totals.InputTokens)
	}
	if totals.OutputTokens != 25 {
		t.Errorf("OutputTokens = %d, want 25", totals.OutputTokens)
	}
}

// TestStreamingLatestUnfinishedKept: when the most recent row is unfinished
// (still streaming), it IS counted so users see live progress.
func TestStreamingLatestUnfinishedKept(t *testing.T) {
	lines := []string{
		`{"type":"assistant","sessionId":"x","timestamp":"2026-05-05T10:00:00Z","message":{"stop_reason":"end_turn","usage":{"input_tokens":100,"output_tokens":25}}}`,
		`{"type":"assistant","sessionId":"x","timestamp":"2026-05-05T10:01:00Z","message":{"stop_reason":null,"usage":{"input_tokens":200,"output_tokens":5}}}`,
	}
	totals, _ := Read(writeTranscript(t, lines))
	if totals.InputTokens != 300 {
		t.Errorf("InputTokens = %d, want 300 (last unfinished must count)", totals.InputTokens)
	}
}

func TestSkipsRowsWithoutUsage(t *testing.T) {
	// permission-mode rows, file-history-snapshot rows, etc. have no usage.
	lines := []string{
		`{"type":"permission-mode","sessionId":"x"}`,
		`{"type":"file-history-snapshot","sessionId":"x","payload":{"foo":"bar"}}`,
	}
	totals, err := Read(writeTranscript(t, lines))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if totals.NumAPICalls != 0 {
		t.Errorf("NumAPICalls = %d, want 0 (no rows with usage)", totals.NumAPICalls)
	}
}

func TestPredominantModel(t *testing.T) {
	lines := []string{
		`{"type":"assistant","sessionId":"x","timestamp":"2026-05-05T10:00:00Z","message":{"model":"claude-opus-4-7","stop_reason":"end_turn","usage":{"input_tokens":100,"output_tokens":50}}}`,
		`{"type":"assistant","sessionId":"x","timestamp":"2026-05-05T10:01:00Z","message":{"model":"claude-haiku-4-5","stop_reason":"end_turn","usage":{"input_tokens":50,"output_tokens":20}}}`,
		`{"type":"assistant","sessionId":"x","timestamp":"2026-05-05T10:02:00Z","message":{"model":"claude-opus-4-7","stop_reason":"end_turn","usage":{"input_tokens":80,"output_tokens":30}}}`,
	}
	totals, err := Read(writeTranscript(t, lines))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if totals.PredominantModel != "claude-opus-4-7" {
		t.Errorf("PredominantModel = %q, want claude-opus-4-7 (2 calls vs haiku 1)", totals.PredominantModel)
	}
	if totals.ModelCounts["claude-opus-4-7"] != 2 {
		t.Errorf("ModelCounts[opus] = %d, want 2", totals.ModelCounts["claude-opus-4-7"])
	}
	if totals.ModelCounts["claude-haiku-4-5"] != 1 {
		t.Errorf("ModelCounts[haiku] = %d, want 1", totals.ModelCounts["claude-haiku-4-5"])
	}
}

func TestMissingFile(t *testing.T) {
	if _, err := Read("/nonexistent/path/sess.jsonl"); err == nil {
		t.Error("expected error for missing file")
	}
}
