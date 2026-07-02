package sessioncost

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAppendLoadDedupKeepsMaxCostPerSession(t *testing.T) {
	t.Setenv("TD_SESSION_COSTS", filepath.Join(t.TempDir(), "session-costs.jsonl"))

	base := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	// Session A climbs; a later under-reporting sample (the /resume case) must
	// not lower A's recorded cost.
	must(t, Append(Sample{SessionID: "a", CostUSD: 1.00, UpdatedAt: base}))
	must(t, Append(Sample{SessionID: "a", CostUSD: 2.50, UpdatedAt: base.Add(time.Minute)}))
	must(t, Append(Sample{SessionID: "a", CostUSD: 0.10, UpdatedAt: base.Add(2 * time.Minute)}))
	// A separate session is summed independently.
	must(t, Append(Sample{SessionID: "b", CostUSD: 0.75, UpdatedAt: base}))

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d sessions, want 2", len(got))
	}
	if got["a"].CostUSD != 2.50 {
		t.Errorf("session a = %.2f, want 2.50 (max, not last)", got["a"].CostUSD)
	}
	if got["b"].CostUSD != 0.75 {
		t.Errorf("session b = %.2f, want 0.75", got["b"].CostUSD)
	}
}

func TestLoadMissingStoreIsEmptyNotError(t *testing.T) {
	t.Setenv("TD_SESSION_COSTS", filepath.Join(t.TempDir(), "does-not-exist.jsonl"))
	got, err := Load()
	if err != nil {
		t.Fatalf("missing store should not error, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want empty map, got %d entries", len(got))
	}
}

func TestCompactCollapsesToLatestPerSession(t *testing.T) {
	t.Setenv("TD_SESSION_COSTS", filepath.Join(t.TempDir(), "session-costs.jsonl"))
	base := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		must(t, Append(Sample{SessionID: "a", CostUSD: float64(i), UpdatedAt: base.Add(time.Duration(i) * time.Minute)}))
	}
	if err := Compact(); err != nil {
		t.Fatalf("Compact: %v", err)
	}
	got, err := Load()
	if err != nil {
		t.Fatalf("Load after compact: %v", err)
	}
	if len(got) != 1 || got["a"].CostUSD != 4 {
		t.Errorf("after compact got %+v, want single session a=4", got)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("append: %v", err)
	}
}
