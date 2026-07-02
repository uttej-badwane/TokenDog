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
	"sort"
	"strings"
	"time"

	"tokendog/internal/analytics"
	"tokendog/internal/pricing"
	"tokendog/internal/sessioncost"
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
	// EarliestLabel is the formatted date of the oldest usage row in the local
	// Claude logs (e.g. "Jun 1, 2026"), or "" when none. "Lifetime" only spans
	// what those logs still contain (Claude Code prunes old sessions), so the UI
	// labels the all-time figure honestly as "since <EarliestLabel>" rather than
	// implying true all-time spend. Pre-formatted to keep the JSON date-free.
	EarliestLabel string `json:"earliest_label"`
	// ByModelToday is today's spend split per model family (Opus/Sonnet/Haiku),
	// highest first — the breakdown a developer actually wants at a glance.
	ByModelToday []ModelSpend `json:"by_model_today"`
	// TokensToday is today's raw token usage across all models.
	TokensToday TokenBlock `json:"tokens_today"`
	// Daily is the last 7 days of spend, most-recent first, each with a
	// display-ready label ("Today", "Yesterday", "Fri 13").
	Daily []DaySpend `json:"daily"`
}

// DaySpend is one calendar day's USD spend with a display label.
type DaySpend struct {
	Date  string  `json:"date"`  // local YYYY-MM-DD
	Label string  `json:"label"` // "Today" / "Yesterday" / "Mon 9"
	USD   float64 `json:"usd"`
}

// ModelSpend is one model family's USD spend for a period.
type ModelSpend struct {
	Model string  `json:"model"`
	USD   float64 `json:"usd"`
}

// TokenBlock is a raw token count split by kind.
type TokenBlock struct {
	Input         int `json:"input"`
	Output        int `json:"output"`
	CacheRead     int `json:"cache_read"`
	CacheCreation int `json:"cache_creation"`
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

const schemaVersion = 2

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

	// Claude Code's own per-session cost (cost.total_cost_usd, the /cost number),
	// captured via the statusLine shim. When present for a session it is
	// authoritative: we scale that session's token-priced rows so the session
	// sums to Claude Code's figure while day/model/token buckets stay
	// proportionally intact. Empty (no shim installed yet) ⇒ pure token pricing.
	captured, _ := sessioncost.Load()
	if len(captured) > 0 {
		_ = sessioncost.Compact() // bound the append-only log; best-effort
	}

	// Parsed rows are served from a disk cache keyed by each file's size+modtime,
	// so repeated invocations (the menu bar polls on a timer) re-parse only the
	// transcripts that actually changed instead of the whole tree.
	reader := loadCache()
	seen := make(map[string]struct{})
	byModel := make(map[string]float64)
	daily := make(map[string]float64)
	var tokens TokenBlock
	var earliest time.Time

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
		// The transcript filename is the session id; if Claude Code captured a
		// cost for it, derive a scale factor from this session's token-priced
		// total so its rows re-price to Claude Code's authoritative number.
		factor := 1.0
		if s, ok := captured[strings.TrimSuffix(d.Name(), ".jsonl")]; ok {
			var tokenTotal float64
			for _, e := range entries {
				tokenTotal += priceEntry(e)
			}
			if tokenTotal > 0 {
				factor = s.CostUSD / tokenTotal
			}
		}
		for _, e := range entries {
			cost := priceEntry(e) * factor
			block.Lifetime += cost
			if e.Timestamp.IsZero() {
				continue // counts toward lifetime, but no day/month bucket
			}
			if earliest.IsZero() || e.Timestamp.Before(earliest) {
				earliest = e.Timestamp
			}
			ts := e.Timestamp.Local()
			daily[ts.Format("2006-01-02")] += cost
			if !ts.Before(monthStart) {
				block.Month += cost
			}
			if !ts.Before(dayStart) {
				block.Today += cost
				byModel[shortModel(e.Model)] += cost
				tokens.Input += e.Input
				tokens.Output += e.Output
				tokens.CacheRead += e.CacheRead
				tokens.CacheCreation += e.CacheCreation
			}
		}
		return nil
	})

	block.TokensToday = tokens
	block.ByModelToday = sortModelSpend(byModel)
	block.Daily = lastSevenDays(now, daily)
	if !earliest.IsZero() {
		block.EarliestLabel = earliest.Local().Format("Jan 2, 2006")
	}

	reader.prune(seen)
	reader.save()
	return block, walkErr
}

// shortModel collapses a versioned model id (e.g. "claude-opus-4-7") to its
// family label for display. Unknown ids pass through unchanged.
func shortModel(model string) string {
	m := strings.ToLower(model)
	switch {
	case m == "":
		return "Unknown"
	case strings.Contains(m, "opus"):
		return "Opus"
	case strings.Contains(m, "sonnet"):
		return "Sonnet"
	case strings.Contains(m, "haiku"):
		return "Haiku"
	default:
		return model
	}
}

// lastSevenDays builds the trailing-7-day series ending today, most-recent
// first, with display-ready labels. Days with no usage are included as $0 so
// the breakdown reads as a continuous strip.
func lastSevenDays(now time.Time, byDay map[string]float64) []DaySpend {
	out := make([]DaySpend, 0, 7)
	start := startOfDay(now)
	for i := 0; i < 7; i++ {
		d := start.AddDate(0, 0, -i)
		key := d.Format("2006-01-02")
		var label string
		switch i {
		case 0:
			label = "Today"
		case 1:
			label = "Yesterday"
		default:
			label = d.Format("Mon 2")
		}
		out = append(out, DaySpend{Date: key, Label: label, USD: byDay[key]})
	}
	return out
}

// sortModelSpend turns the per-model map into a slice ordered by spend
// descending (ties broken by name) so the UI renders a stable, ranked list.
func sortModelSpend(m map[string]float64) []ModelSpend {
	out := make([]ModelSpend, 0, len(m))
	for k, v := range m {
		out = append(out, ModelSpend{Model: k, USD: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].USD != out[j].USD {
			return out[i].USD > out[j].USD
		}
		return out[i].Model < out[j].Model
	})
	return out
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
