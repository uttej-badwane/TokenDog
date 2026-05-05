package calibration

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tokendog/internal/analytics"
)

func writeTranscript(t *testing.T, lines []string) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "transcript.jsonl")
	if err := os.WriteFile(f, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	return f
}

// Standard "session-with-known-totals" fixture: 1000 input + 5000 cache_creation
// finalized in one API call.
func sessionFixture(t *testing.T, sessionID string) string {
	return writeTranscript(t, []string{
		`{"type":"assistant","sessionId":"` + sessionID + `","timestamp":"2026-05-05T10:00:00Z","message":{"stop_reason":"end_turn","usage":{"input_tokens":1000,"cache_creation_input_tokens":5000,"output_tokens":50}}}`,
	})
}

func TestRecomputeMatchesSessions(t *testing.T) {
	tp1 := sessionFixture(t, "sess-1")
	tp2 := sessionFixture(t, "sess-2")
	tp3 := sessionFixture(t, "sess-3")

	records := []analytics.Record{
		// sess-1: 2000 cl100k from td → ratio = (1000+5000) / 2000 = 3.0
		{SessionID: "sess-1", TranscriptPath: tp1, RawTokens: 1000, Timestamp: time.Now()},
		{SessionID: "sess-1", TranscriptPath: tp1, RawTokens: 1000, Timestamp: time.Now()},
		// sess-2: 3000 cl100k → ratio = 6000/3000 = 2.0
		{SessionID: "sess-2", TranscriptPath: tp2, RawTokens: 3000, Timestamp: time.Now()},
		// sess-3: 6000 cl100k → ratio = 6000/6000 = 1.0
		{SessionID: "sess-3", TranscriptPath: tp3, RawTokens: 6000, Timestamp: time.Now()},
	}

	snap, err := Recompute(records)
	if err != nil {
		t.Fatalf("Recompute: %v", err)
	}
	if len(snap.Sessions) != 3 {
		t.Fatalf("want 3 sessions, got %d", len(snap.Sessions))
	}
	// Weighted ratio = (6000+6000+6000) / (2000+3000+6000) = 18000/11000 ≈ 1.636
	want := 18000.0 / 11000.0
	if math.Abs(snap.Ratio-want) > 0.001 {
		t.Errorf("Ratio = %v, want %v", snap.Ratio, want)
	}
}

func TestRecomputeIgnoresRecordsWithoutSession(t *testing.T) {
	records := []analytics.Record{
		{SessionID: "", TranscriptPath: "/somewhere", RawTokens: 1000, Timestamp: time.Now()},
		{SessionID: "x", TranscriptPath: "", RawTokens: 1000, Timestamp: time.Now()},
	}
	snap, err := Recompute(records)
	if err != nil {
		t.Fatalf("Recompute: %v", err)
	}
	if len(snap.Sessions) != 0 {
		t.Errorf("want 0 sessions (no usable records), got %d", len(snap.Sessions))
	}
	if snap.Ratio != 0 {
		t.Errorf("want ratio 0 with no samples, got %v", snap.Ratio)
	}
}

func TestApplyBelowMinSamples(t *testing.T) {
	s := &Snapshot{
		Sessions: []SessionSample{{Ratio: 1.5}},
		Ratio:    1.5,
	}
	if got := s.Apply(10.0); got != 10.0 {
		t.Errorf("Apply with 1 sample = %v, want unchanged 10.0", got)
	}
}

func TestApplyAboveMinSamples(t *testing.T) {
	s := &Snapshot{
		Sessions: []SessionSample{{}, {}, {}, {}},
		Ratio:    1.5,
	}
	if got := s.Apply(10.0); math.Abs(got-15.0) > 0.001 {
		t.Errorf("Apply with 4 samples and ratio 1.5 = %v, want 15.0", got)
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	orig := &Snapshot{
		Sessions: []SessionSample{{SessionID: "s", Ratio: 1.4, AnthropicInput: 100, CL100KFromTD: 70}},
		Ratio:    1.4,
		LastComputed: time.Now().UTC().Truncate(time.Second),
	}
	if err := Save(orig); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Sessions) != 1 || loaded.Ratio != 1.4 {
		t.Errorf("roundtrip mismatch: %+v", loaded)
	}
}

func TestLoadMissingFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	s, err := Load()
	if err != nil {
		t.Fatalf("Load missing: %v", err)
	}
	if s == nil || len(s.Sessions) != 0 {
		t.Errorf("want empty snapshot, got %+v", s)
	}
}
