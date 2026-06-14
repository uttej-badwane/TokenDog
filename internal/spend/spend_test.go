package spend

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"tokendog/internal/pricing"
)

// writeTranscript writes a single-line transcript with one finalized usage row
// at the given time, using a known model. Each row carries a stop_reason so the
// dedup path treats it as a finalized (counted) entry.
func writeTranscript(t *testing.T, dir, name, model string, ts time.Time, in, out, cacheRead, cacheCreate int) {
	t.Helper()
	line := fmt.Sprintf(`{"type":"assistant","sessionId":"s","timestamp":%q,"message":{"model":%q,"stop_reason":"end_turn","usage":{"input_tokens":%d,"output_tokens":%d,"cache_read_input_tokens":%d,"cache_creation_input_tokens":%d}}}`,
		ts.Format(time.RFC3339Nano), model, in, out, cacheRead, cacheCreate)
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(line+"\n"), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
}

func TestComputeSpendBuckets(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TD_CLAUDE_PROJECTS", dir)

	now := time.Now()
	model := "claude-opus-4-7"

	// One row at `now` (today + this month) and one clearly in a previous month
	// (lifetime only). Anchoring on `now` and `startOfMonth(now)-24h` keeps the
	// buckets deterministic regardless of the calendar day the test runs on.
	const in, out = 1_000_000, 1_000_000
	writeTranscript(t, dir, "today.jsonl", model, now, in, out, 0, 0)
	writeTranscript(t, dir, "lastmonth.jsonl", model, startOfMonth(now).Add(-24*time.Hour), in, out, 0, 0)

	block, err := computeSpend(now)
	if err != nil {
		t.Fatalf("computeSpend: %v", err)
	}
	if !block.Available {
		t.Fatal("expected Available=true when log dir exists")
	}

	r, _ := pricing.Lookup(model)
	perRow := float64(in)/1e6*r.InputPerM + float64(out)/1e6*r.OutputPerM

	if got, want := block.Lifetime, 2*perRow; !almostEqual(got, want) {
		t.Errorf("lifetime = %.4f, want %.4f (both rows)", got, want)
	}
	if got, want := block.Month, perRow; !almostEqual(got, want) {
		t.Errorf("month = %.4f, want %.4f (last-month row excluded)", got, want)
	}
	if got, want := block.Today, perRow; !almostEqual(got, want) {
		t.Errorf("today = %.4f, want %.4f (only the now row)", got, want)
	}
}

func TestComputeSpendMissingDir(t *testing.T) {
	t.Setenv("TD_CLAUDE_PROJECTS", filepath.Join(t.TempDir(), "does-not-exist"))
	block, err := computeSpend(time.Now())
	if err != nil {
		t.Fatalf("missing dir should not error, got %v", err)
	}
	if block.Available {
		t.Error("expected Available=false for a missing log dir")
	}
	if block.Lifetime != 0 {
		t.Errorf("expected zero spend for missing dir, got %.4f", block.Lifetime)
	}
}

func TestComputeReport(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TD_CLAUDE_PROJECTS", dir)
	writeTranscript(t, dir, "a.jsonl", "claude-opus-4-7", time.Now(), 1_000_000, 0, 0, 0)

	rep, err := Compute("test-1.2.3")
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if rep.Schema != schemaVersion {
		t.Errorf("schema = %d, want %d", rep.Schema, schemaVersion)
	}
	if rep.TDVersion != "test-1.2.3" {
		t.Errorf("td_version = %q, want test-1.2.3", rep.TDVersion)
	}
	if rep.Spend.Currency != "USD" {
		t.Errorf("currency = %q, want USD", rep.Spend.Currency)
	}
	if rep.Spend.Lifetime <= 0 {
		t.Errorf("expected positive lifetime spend, got %.4f", rep.Spend.Lifetime)
	}
	// SharePct must be in [0,100] whenever it is set.
	if rep.SharePct < 0 || rep.SharePct > 100 {
		t.Errorf("share_pct out of range: %.4f", rep.SharePct)
	}
}

func almostEqual(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 1e-6
}
