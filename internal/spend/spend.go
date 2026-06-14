// Package spend computes Claude API spend from the usage logs Claude Code
// writes under ~/.claude/projects/**/*.jsonl, and joins it with TokenDog's
// own savings. It is the data source behind `td spend` and the macOS menu-bar
// app.
//
// Spend is priced natively (no ccusage / npx dependency) by reading each
// transcript's per-message token usage via internal/transcript and applying
// per-model rates from internal/pricing — the standard usage-cost model:
// input + output + cache_read + cache_creation, each at its own per-million
// rate. Everything here is pure Go so the `td` binary stays CGO-free.
package spend

import (
	"os"
	"path/filepath"
	"time"

	"tokendog/internal/analytics"
	"tokendog/internal/pricing"
	"tokendog/internal/transcript"
)

// SpendBlock holds USD spend bucketed by period. Currency is always USD for
// now; the field exists so the JSON contract can grow without a schema bump.
type SpendBlock struct {
	Today     float64 `json:"today"`
	Month     float64 `json:"month"`
	Lifetime  float64 `json:"lifetime"`
	Currency  string  `json:"currency"`
	Available bool    `json:"available"`
}

// SavedBlock holds TokenDog's own savings, for the "TD saved of it" line.
type SavedBlock struct {
	Today    float64 `json:"today"`
	Lifetime float64 `json:"lifetime"`
	Tokens   int     `json:"tokens"`
}

// Report is the full menu-bar contract emitted by `td spend --json`. Schema
// is versioned so the Swift client can detect an incompatible td.
type Report struct {
	Schema      int        `json:"schema"`
	GeneratedAt time.Time  `json:"generated_at"`
	Spend       SpendBlock `json:"spend"`
	Saved       SavedBlock `json:"saved"`
	SharePct    float64    `json:"share_pct"`
	TDVersion   string     `json:"td_version"`
}

const schemaVersion = 1

// logDir returns the directory Claude Code writes session transcripts to.
// Overridable via TD_CLAUDE_PROJECTS for tests and non-standard installs.
func logDir() (string, error) {
	if d := os.Getenv("TD_CLAUDE_PROJECTS"); d != "" {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "projects"), nil
}

// priceEntry returns the USD cost of one usage row, priced at the rate that was
// in effect at the row's timestamp. Unknown models resolve to
// pricing.DefaultModel (LookupAt never returns a nil rate), so a missing model
// is priced conservatively rather than dropped.
func priceEntry(e transcript.Entry) float64 {
	r, _ := pricing.LookupAt(e.Model, e.Timestamp)
	const m = 1_000_000.0
	return float64(e.Input)/m*r.InputPerM +
		float64(e.Output)/m*r.OutputPerM +
		float64(e.CacheRead)/m*r.CacheReadPerM +
		float64(e.CacheCreation)/m*r.CacheWritePerM
}

// computeSpend walks the log dir and buckets priced usage into today / month /
// lifetime using each row's local-time timestamp. A missing log dir yields a
// zero block with Available=false (not an error): the menu bar then falls back
// to showing savings only.
func computeSpend(now time.Time) (SpendBlock, error) {
	block := SpendBlock{Currency: "USD"}
	dir, err := logDir()
	if err != nil {
		return block, err
	}
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return block, nil // Available stays false
		}
		return block, err
	}
	block.Available = true

	dayStart := startOfDay(now)
	monthStart := startOfMonth(now)

	// Parsed rows are served from a disk cache keyed by each file's size+modtime,
	// so repeated invocations (the menu bar polls on a timer) re-parse only the
	// transcripts that actually changed instead of the whole tree.
	reader := loadCache()
	seen := make(map[string]struct{})

	walkErr := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable subtrees rather than abort the whole scan
		}
		if d.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		seen[path] = struct{}{}
		entries, err := reader.entries(path, d)
		if err != nil {
			return nil // one bad transcript shouldn't sink the total
		}
		for _, e := range entries {
			cost := priceEntry(e)
			block.Lifetime += cost
			if e.Timestamp.IsZero() {
				continue // counts toward lifetime, but no day/month bucket
			}
			ts := e.Timestamp.Local()
			if !ts.Before(monthStart) {
				block.Month += cost
			}
			if !ts.Before(dayStart) {
				block.Today += cost
			}
		}
		return nil
	})

	reader.prune(seen)
	reader.save()
	return block, walkErr
}

// computeSaved derives TokenDog's savings from its analytics history: lifetime
// from the full summary, today from records on or after local midnight. Models
// are resolved first so USD figures use real per-model rates.
func computeSaved(now time.Time) (SavedBlock, error) {
	records, err := analytics.LoadAll()
	if err != nil {
		return SavedBlock{}, err
	}
	analytics.ResolveModels(records)

	lifetime, _ := analytics.Summarize(records)

	dayStart := startOfDay(now)
	today := records[:0:0]
	for _, r := range records {
		if !r.Timestamp.Before(dayStart) {
			today = append(today, r)
		}
	}
	todaySummary, _ := analytics.Summarize(today)

	return SavedBlock{
		Today:    todaySummary.USDSaved(),
		Lifetime: lifetime.USDSaved(),
		Tokens:   lifetime.TokensSaved(),
	}, nil
}

// Compute builds the full report. Spend and savings are computed independently
// so one failing source still yields a usable report for the other.
func Compute(version string) (Report, error) {
	now := time.Now()
	rep := Report{
		Schema:      schemaVersion,
		GeneratedAt: now.UTC(),
		TDVersion:   version,
	}

	sp, spErr := computeSpend(now)
	rep.Spend = sp

	sv, svErr := computeSaved(now)
	rep.Saved = sv

	// Share of the bill TokenDog clawed back: saved / (spend + saved). Using
	// lifetime on both sides keeps the ratio stable across a session.
	if denom := rep.Spend.Lifetime + rep.Saved.Lifetime; denom > 0 {
		rep.SharePct = rep.Saved.Lifetime / denom * 100
	}

	if spErr != nil {
		return rep, spErr
	}
	return rep, svErr
}

func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func startOfMonth(t time.Time) time.Time {
	y, m, _ := t.Date()
	return time.Date(y, m, 1, 0, 0, 0, 0, t.Location())
}
