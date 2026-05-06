package analytics

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"tokendog/internal/pricing"
	"tokendog/internal/tokenizer"
	"tokendog/internal/transcript"
)

// Cost math lives in internal/pricing — RenderGain sums per-model rates via
// Summary.ByModel. The earlier Calibrator interface was removed in v0.7.0
// after we found its multiplier was measuring session-token-density rather
// than tokenizer accuracy and overstating USD by 100x+ in real workloads.

type Record struct {
	Command        string    `json:"command"`
	Timestamp      time.Time `json:"timestamp"`
	RawBytes       int       `json:"raw_bytes"`
	FilteredBytes  int       `json:"filtered_bytes"`
	RawTokens      int       `json:"raw_tokens,omitempty"`
	FilteredTokens int       `json:"filtered_tokens,omitempty"`
	DurationMs     int64     `json:"duration_ms"`
	CacheHit       bool      `json:"cache_hit,omitempty"`
	SessionID      string    `json:"session_id,omitempty"`
	TranscriptPath string    `json:"transcript_path,omitempty"`
	// Model is populated lazily at gain/replay time by reading the
	// transcript at TranscriptPath and taking PredominantModel. Stored
	// when first resolved so the same lookup isn't repeated. Empty for
	// records whose transcript was unreachable.
	Model string `json:"model,omitempty"`
}

func (r Record) BytesSaved() int { return r.RawBytes - r.FilteredBytes }

// TokensSaved prefers the tokenizer-computed counts when present, falling
// back to a bytes/4 estimate for legacy records written before the
// tokenizer existed.
func (r Record) TokensSaved() int {
	if r.RawTokens > 0 || r.FilteredTokens > 0 {
		return r.RawTokens - r.FilteredTokens
	}
	return EstimateTokens(r.BytesSaved())
}

func (r Record) SavedPct() float64 {
	if r.RawBytes == 0 {
		return 0
	}
	return float64(r.BytesSaved()) / float64(r.RawBytes) * 100
}

func EstimateTokens(bytes int) int { return (bytes + 3) / 4 }

// NewRecord computes raw + filtered token counts via the real tokenizer and
// returns a fully-populated Record ready for Save. Bytes are still tracked
// (cheap, deterministic) but tokens are now the primary metric. Session
// fields are read from TD_SESSION_ID + TD_TRANSCRIPT_PATH env vars if set —
// the hook injects these so each `td <tool>` exec inherits Claude's session
// context.
func NewRecord(command string, raw, filtered string, durationMs int64) Record {
	r := Record{
		Command:        command,
		Timestamp:      time.Now(),
		RawBytes:       len(raw),
		FilteredBytes:  len(filtered),
		RawTokens:      tokenizer.Count(raw),
		FilteredTokens: tokenizer.Count(filtered),
		DurationMs:     durationMs,
	}
	addSessionEnv(&r)
	return r
}

// NewCacheHitRecord builds a record for a cache hit. The raw side is what
// the prior call produced (we only stored its byte count, so tokens are
// estimated); the filtered side is the short marker actually emitted to the
// model.
func NewCacheHitRecord(command string, rawBytes int, marker string) Record {
	r := Record{
		Command:        command,
		Timestamp:      time.Now(),
		RawBytes:       rawBytes,
		FilteredBytes:  len(marker),
		RawTokens:      EstimateTokens(rawBytes),
		FilteredTokens: tokenizer.Count(marker),
		DurationMs:     0,
		CacheHit:       true,
	}
	addSessionEnv(&r)
	return r
}

func addSessionEnv(r *Record) {
	r.SessionID = os.Getenv("TD_SESSION_ID")
	r.TranscriptPath = os.Getenv("TD_TRANSCRIPT_PATH")
}

// ResolveModels populates r.Model in-place for any records that have a
// SessionID + TranscriptPath but no Model yet. Reads each transcript once
// (cached by SessionID) and uses PredominantModel. Records that already
// carry a Model are left alone. Errors per-transcript are silent — the
// record stays Model="" and is bucketed as "unknown" downstream.
//
// This is called at gain/replay time, NOT at live exec — populating Model
// during the hook would require reading the transcript on every Bash call,
// which is too slow.
func ResolveModels(records []Record) {
	cache := map[string]string{}
	for i := range records {
		r := &records[i]
		if r.Model != "" || r.SessionID == "" || r.TranscriptPath == "" {
			continue
		}
		if m, ok := cache[r.SessionID]; ok {
			r.Model = m
			continue
		}
		t, err := transcript.Read(r.TranscriptPath)
		if err != nil {
			cache[r.SessionID] = "" // negative cache so we don't retry
			continue
		}
		m := t.PredominantModel
		cache[r.SessionID] = m
		r.Model = m
	}
}

func dataPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".config", "tokendog")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "history.jsonl"), nil
}

// rotation thresholds — kept generous so casual users never hit them.
const (
	maxHistoryRecords = 100_000
	maxHistoryAge     = 90 * 24 * time.Hour
	rotateCheckEvery  = 256 // only check size every Nth save (cheap)
)

// saveCounter is incremented on every Save and triggers rotation checks
// every rotateCheckEvery writes — avoids per-save os.Stat() overhead while
// still catching the rotation threshold within a few hundred commands.
var saveCounter int

func Save(r Record) error {
	path, err := dataPath()
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(r); err != nil {
		return err
	}
	saveCounter++
	if saveCounter%rotateCheckEvery == 0 {
		go maybeRotate(path)
	}
	return nil
}

// maybeRotate moves history.jsonl to history-YYYY-MM.jsonl.gz if it has
// crossed maxHistoryRecords or contains records older than maxHistoryAge.
// Best-effort: errors are swallowed because analytics persistence must
// never block the user's command. Runs in a goroutine from Save.
func maybeRotate(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	// Quick byte-size heuristic: 100K records × ~300 bytes = ~30MB. If the
	// file is well under that, skip the line count.
	if info.Size() < 5*1024*1024 {
		return
	}
	records, err := loadAllFromPath(path)
	if err != nil || len(records) == 0 {
		return
	}
	tooMany := len(records) >= maxHistoryRecords
	tooOld := time.Since(records[0].Timestamp) > maxHistoryAge
	if !tooMany && !tooOld {
		return
	}
	// Archive: split off everything older than 30 days, gzip it under a
	// dated name, leave the recent slice in place.
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	var keep, archive []Record
	for _, rec := range records {
		if rec.Timestamp.Before(cutoff) {
			archive = append(archive, rec)
		} else {
			keep = append(keep, rec)
		}
	}
	if len(archive) == 0 {
		return
	}
	archivePath := path + "-" + time.Now().Format("2006-01") + ".jsonl.gz"
	if err := writeArchive(archivePath, archive); err != nil {
		return
	}
	// Rewrite history.jsonl with only the kept slice. Use a temp-file +
	// rename so a crash mid-write can't corrupt history.
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	enc := json.NewEncoder(f)
	for _, rec := range keep {
		_ = enc.Encode(rec)
	}
	f.Close()
	_ = os.Rename(tmp, path)
}

func loadAllFromPath(path string) ([]Record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var records []Record
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		var r Record
		if json.Unmarshal(sc.Bytes(), &r) == nil {
			records = append(records, r)
		}
	}
	return records, sc.Err()
}

func writeArchive(path string, records []Record) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		// Archive for this month already exists — append to it.
		f, err = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	enc := json.NewEncoder(gz)
	for _, r := range records {
		if err := enc.Encode(r); err != nil {
			return err
		}
	}
	return nil
}

func LoadAll() ([]Record, error) {
	path, err := dataPath()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var records []Record
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var r Record
		if json.Unmarshal(scanner.Bytes(), &r) == nil {
			records = append(records, r)
		}
	}
	return records, scanner.Err()
}

type Summary struct {
	TotalCommands       int
	TotalRawBytes       int
	TotalFilteredBytes  int
	TotalRawTokens      int
	TotalFilteredTokens int
	TotalTokensSaved    int // pre-aggregated per-record (handles mixed legacy + tokenized history)
	TotalDurationMs     int64
	CacheHits           int
	ByModel             map[string]*ModelStat // per-model aggregation, populated by Summarize
}

// ModelStat holds per-model totals for the breakdown view. The Model name
// is the canonical id matching pricing.Lookup; empty means "unknown" (no
// transcript was reachable to resolve it).
type ModelStat struct {
	Model       string
	Commands    int
	TokensSaved int
	BytesSaved  int
	USDSaved    float64 // computed using pricing.Lookup at render time
	IsImputed   bool    // true when pricing fell back to default
}

func (s Summary) BytesSaved() int  { return s.TotalRawBytes - s.TotalFilteredBytes }
func (s Summary) TokensSaved() int { return s.TotalTokensSaved }

// USDSaved sums per-model USD savings using each model's actual input rate
// (resolved via internal/pricing). Records with no resolved model land in
// the "unknown" bucket which is priced at DefaultModel as a conservative
// upper bound.
func (s Summary) USDSaved() float64 {
	total := 0.0
	for _, ms := range s.ByModel {
		total += ms.USDSaved
	}
	return total
}

func (s Summary) SavedPct() float64 {
	if s.TotalRawBytes == 0 {
		return 0
	}
	return float64(s.BytesSaved()) / float64(s.TotalRawBytes) * 100
}

type CommandStat struct {
	Name        string
	Count       int
	Saved       int
	TokensSaved int
	AvgPct      float64
	AvgMs       int64
}

func Summarize(records []Record) (Summary, []CommandStat) {
	var s Summary
	s.ByModel = map[string]*ModelStat{}
	byCmd := map[string]*CommandStat{}

	for _, r := range records {
		s.TotalCommands++
		s.TotalRawBytes += r.RawBytes
		s.TotalFilteredBytes += r.FilteredBytes
		s.TotalRawTokens += r.RawTokens
		s.TotalFilteredTokens += r.FilteredTokens
		// Per-record tokens-saved falls back to bytes/4 for legacy records
		// that predate the tokenizer. Aggregating per-record correctly
		// handles mixed history; a global fallback would double-count.
		ts := r.TokensSaved()
		s.TotalTokensSaved += ts
		s.TotalDurationMs += r.DurationMs
		if r.CacheHit {
			s.CacheHits++
		}

		// Per-model bucket. Empty model goes into "unknown" so the row
		// shows up in --by-model as a distinct category rather than
		// silently merging into the default-priced bucket.
		modelKey := r.Model
		if modelKey == "" {
			modelKey = "unknown"
		}
		ms, ok := s.ByModel[modelKey]
		if !ok {
			rate, hit := pricing.Lookup(modelKey)
			ms = &ModelStat{Model: rate.Model, IsImputed: !hit}
			if modelKey == "unknown" {
				ms.Model = "unknown"
				ms.IsImputed = true
			}
			s.ByModel[modelKey] = ms
		}
		ms.Commands++
		ms.TokensSaved += ts
		ms.BytesSaved += r.BytesSaved()

		name := normalizeName(r.Command)
		cs, ok := byCmd[name]
		if !ok {
			cs = &CommandStat{Name: name}
			byCmd[name] = cs
		}
		cs.Count++
		cs.Saved += r.BytesSaved()
		cs.TokensSaved += ts
		cs.AvgPct = (cs.AvgPct*float64(cs.Count-1) + r.SavedPct()) / float64(cs.Count)
		cs.AvgMs = (cs.AvgMs*int64(cs.Count-1) + r.DurationMs) / int64(cs.Count)
	}

	// Compute per-model USD using each model's input rate. unknown buckets
	// use the DefaultModel rate but stay flagged as imputed.
	for _, ms := range s.ByModel {
		key := ms.Model
		if key == "unknown" {
			key = pricing.DefaultModel
		}
		rate, _ := pricing.Lookup(key)
		ms.USDSaved = rate.USDForInput(ms.TokensSaved)
	}

	stats := make([]CommandStat, 0, len(byCmd))
	for _, cs := range byCmd {
		stats = append(stats, *cs)
	}
	sort.Slice(stats, func(i, j int) bool { return stats[i].Saved > stats[j].Saved })
	return s, stats
}

func normalizeName(cmd string) string {
	cmd = strings.TrimPrefix(cmd, "td ")
	cmd = strings.TrimPrefix(cmd, "tokendog ")
	parts := strings.Fields(cmd)
	if len(parts) >= 2 {
		return parts[0] + " " + parts[1]
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return cmd
}

func RenderGain(records []Record, showHistory bool) string {
	if len(records) == 0 {
		return "No data yet. Run td commands to start tracking savings.\n"
	}

	summary, stats := Summarize(records)
	var b strings.Builder

	sep60 := strings.Repeat("═", 60)
	sep71 := strings.Repeat("─", 71)

	b.WriteString("TokenDog Savings\n")
	b.WriteString(sep60 + "\n\n")
	b.WriteString(fmt.Sprintf("%-22s %d", "Total commands:", summary.TotalCommands))
	if summary.CacheHits > 0 {
		b.WriteString(fmt.Sprintf(" (%d cache hits)", summary.CacheHits))
	}
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("%-22s %s\n", "Raw output:", humanBytes(summary.TotalRawBytes)))
	b.WriteString(fmt.Sprintf("%-22s %s\n", "After filter:", humanBytes(summary.TotalFilteredBytes)))
	b.WriteString(fmt.Sprintf("%-22s %s (%d tokens, %.1f%%)\n",
		"Saved:", humanBytes(summary.BytesSaved()), summary.TokensSaved(), summary.SavedPct()))

	// Cost line — sums per-model USD using each model's actual rate
	// (resolved at NewRecord time from the transcript). Records without a
	// resolved model fall into the "unknown" bucket priced at DefaultModel
	// (Opus 4.7), which over-estimates rather than under — the savings
	// figure is conservative-low when honest.
	tokenSrc := "estimated"
	if tokenizer.Available() && summary.TotalRawTokens > 0 {
		tokenSrc = "cl100k"
	}
	totalUSD := 0.0
	imputedTokens := 0
	for _, ms := range summary.ByModel {
		totalUSD += ms.USDSaved
		if ms.IsImputed {
			imputedTokens += ms.TokensSaved
		}
	}
	imputedNote := ""
	if imputedTokens > 0 && summary.TokensSaved() > 0 {
		pctImputed := float64(imputedTokens) / float64(summary.TokensSaved()) * 100
		imputedNote = fmt.Sprintf(", %.0f%% priced at default-model fallback", pctImputed)
	}
	b.WriteString(fmt.Sprintf("%-22s $%.4f (per-model rates, tokens via %s%s)\n",
		"Cost saved:", totalUSD, tokenSrc, imputedNote))

	pct := summary.SavedPct()
	b.WriteString(fmt.Sprintf("%-22s %s %.1f%%\n\n", "Efficiency:", progressBar(pct, 24), pct))

	b.WriteString("By Command\n")
	b.WriteString(sep71 + "\n")
	b.WriteString(fmt.Sprintf("  %-3s  %-28s  %-5s  %-8s  %-6s  %-6s  %s\n",
		"#", "Command", "Count", "Saved", "Avg%", "AvgMs", "Impact"))
	b.WriteString(sep71 + "\n")

	maxSaved := 0
	for _, cs := range stats {
		if cs.Saved > maxSaved {
			maxSaved = cs.Saved
		}
	}

	for i, cs := range stats {
		impact := ""
		if maxSaved > 0 {
			impact = progressBar(float64(cs.Saved)/float64(maxSaved)*100, 10)
		}
		name := cs.Name
		if len(name) > 28 {
			name = name[:25] + "..."
		}
		b.WriteString(fmt.Sprintf("  %-3d  %-28s  %-5d  %-8s  %5.1f%%  %4dms  %s\n",
			i+1, name, cs.Count, humanBytes(cs.Saved), cs.AvgPct, cs.AvgMs, impact))
	}
	b.WriteString(sep71 + "\n")

	if showHistory && len(records) > 0 {
		b.WriteString("\nRecent Commands\n")
		b.WriteString(strings.Repeat("─", 60) + "\n")
		start := len(records) - 20
		if start < 0 {
			start = 0
		}
		for _, r := range records[start:] {
			arrow := "•"
			if r.BytesSaved() > 0 {
				arrow = "▲"
			}
			name := r.Command
			if len(name) > 32 {
				name = name[:29] + "..."
			}
			b.WriteString(fmt.Sprintf("%s %s %-34s %4.0f%% (%s)\n",
				r.Timestamp.Format("01-02 15:04"), arrow, name, r.SavedPct(), humanBytes(r.BytesSaved())))
		}
	}

	return b.String()
}

// RenderSessionGain renders the per-session view: TD's savings on that
// session vs. Anthropic's actual token consumption from the transcript.
// `totals` may be nil if no transcript was reachable (env-var injection
// disabled, file deleted, etc.) — in that case we show TD-only stats.
func RenderSessionGain(sessionID string, records []Record, totals *transcript.SessionTotals) string {
	summary, _ := Summarize(records)
	var b strings.Builder

	sep60 := strings.Repeat("═", 60)
	b.WriteString(fmt.Sprintf("TokenDog Savings — session %s\n", shortSessionID(sessionID)))
	b.WriteString(sep60 + "\n\n")

	b.WriteString(fmt.Sprintf("%-26s %d", "TD-filtered commands:", summary.TotalCommands))
	if summary.CacheHits > 0 {
		b.WriteString(fmt.Sprintf(" (%d cache hits)", summary.CacheHits))
	}
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("%-26s %s (%d tokens via cl100k)\n",
		"TD bytes saved:", humanBytes(summary.BytesSaved()), summary.TokensSaved()))

	if totals == nil {
		b.WriteString("\n[transcript unavailable — showing TD-only stats]\n")
		return b.String()
	}

	b.WriteString("\nAnthropic-reported usage (from transcript):\n")
	b.WriteString(fmt.Sprintf("  %-26s %d API calls\n", "Turns:", totals.NumAPICalls))
	b.WriteString(fmt.Sprintf("  %-26s %d tokens\n", "Uncached input:", totals.InputTokens))
	b.WriteString(fmt.Sprintf("  %-26s %d tokens\n", "Cache creation:", totals.CacheCreationTokens))
	b.WriteString(fmt.Sprintf("  %-26s %d tokens\n", "Cache read:", totals.CacheReadTokens))
	b.WriteString(fmt.Sprintf("  %-26s %d tokens\n", "Output:", totals.OutputTokens))
	b.WriteString(fmt.Sprintf("  %-26s %d tokens\n", "Total context (no output):", totals.TotalContextTokens))

	// "TD share" — fraction of newly-seen content this session that TD
	// touched. anthropic_input_new = uncached_input + cache_creation
	// (excludes cache_read, which is repeat reads of already-counted tokens).
	anthNew := totals.InputTokens + totals.CacheCreationTokens
	if anthNew > 0 && summary.TotalRawTokens > 0 {
		// What fraction of newly-seen content was tool output that TD saw?
		share := float64(summary.TotalRawTokens) / float64(anthNew) * 100
		if share > 100 {
			share = 100
		}
		b.WriteString(fmt.Sprintf("\n%-26s %.1f%% of newly-seen tokens were TD-touched tool output\n",
			"TD share of input:", share))
	}

	// USD this session — uses per-model rates from Summary.ByModel. When
	// the predominant model isn't resolvable, falls into the "unknown"
	// bucket priced at DefaultModel as a conservative upper bound.
	usd := summary.USDSaved()
	model := pricing.DefaultModel
	if totals.PredominantModel != "" {
		model = totals.PredominantModel
	}
	b.WriteString(fmt.Sprintf("%-26s $%.4f at %s (cl100k)\n",
		"Estimated cost saved:", usd, model))

	return b.String()
}

// RenderByModel produces a per-model breakdown table. Designed to compose
// under the standard RenderGain output — the headline summary already shows
// blended cost; this section reveals the model mix behind that number.
func RenderByModel(records []Record) string {
	summary, _ := Summarize(records)
	if len(summary.ByModel) == 0 {
		return ""
	}
	var b strings.Builder
	dash := strings.Repeat("─", 72)
	b.WriteString("\nBy Model\n")
	b.WriteString(dash + "\n")
	b.WriteString(fmt.Sprintf("  %-22s  %-8s  %-12s  %-10s  %s\n",
		"Model", "Calls", "Tokens saved", "$ saved", "Note"))
	b.WriteString(dash + "\n")

	type row struct {
		ms *ModelStat
	}
	rows := make([]row, 0, len(summary.ByModel))
	for _, ms := range summary.ByModel {
		rows = append(rows, row{ms: ms})
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].ms.USDSaved > rows[j].ms.USDSaved
	})
	for _, r := range rows {
		note := ""
		if r.ms.IsImputed {
			note = "imputed price"
		}
		b.WriteString(fmt.Sprintf("  %-22s  %-8d  %-12d  $%-9.4f  %s\n",
			r.ms.Model, r.ms.Commands, r.ms.TokensSaved, r.ms.USDSaved, note))
	}
	b.WriteString(dash + "\n")
	return b.String()
}

// TimeBucket is a single row in a daily/monthly aggregation.
type TimeBucket struct {
	Period      string         `json:"period"` // "2026-05-05" or "2026-05"
	Commands    int            `json:"commands"`
	TokensSaved int            `json:"tokens_saved"`
	BytesSaved  int            `json:"bytes_saved"`
	USDSaved    float64        `json:"usd_saved"`
	ByModel     map[string]int `json:"by_model,omitempty"` // model → tokens saved
}

// TimeSeriesData groups records by day or month. ByModel sub-bucketing is
// included only when the caller asks for it (avoids noise in default JSON).
func TimeSeriesData(records []Record, monthly, byModel bool) []TimeBucket {
	buckets := map[string]*TimeBucket{}
	for _, r := range records {
		key := r.Timestamp.UTC().Format("2006-01-02")
		if monthly {
			key = r.Timestamp.UTC().Format("2006-01")
		}
		b, ok := buckets[key]
		if !ok {
			b = &TimeBucket{Period: key}
			if byModel {
				b.ByModel = map[string]int{}
			}
			buckets[key] = b
		}
		ts := r.TokensSaved()
		b.Commands++
		b.TokensSaved += ts
		b.BytesSaved += r.BytesSaved()
		modelKey := r.Model
		if modelKey == "" {
			modelKey = "unknown"
		}
		// Pricing per record; sums correctly when sessions mix models.
		priceModel := modelKey
		if priceModel == "unknown" {
			priceModel = pricing.DefaultModel
		}
		rate, _ := pricing.Lookup(priceModel)
		b.USDSaved += rate.USDForInput(ts)
		if byModel {
			b.ByModel[modelKey] += ts
		}
	}
	out := make([]TimeBucket, 0, len(buckets))
	for _, b := range buckets {
		out = append(out, *b)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Period < out[j].Period
	})
	return out
}

// RenderTimeSeries formats a daily/monthly breakdown as a human-readable
// table. byModel adds a second-row "(model: N tokens)" annotation for each
// period — kept off by default to avoid screen-overload on long histories.
func RenderTimeSeries(records []Record, monthly, byModel bool) string {
	series := TimeSeriesData(records, monthly, byModel)
	if len(series) == 0 {
		return "No data in the requested date range.\n"
	}
	var b strings.Builder
	dash := strings.Repeat("─", 72)
	header := "Daily breakdown"
	if monthly {
		header = "Monthly breakdown"
	}
	b.WriteString(header + "\n")
	b.WriteString(dash + "\n")
	b.WriteString(fmt.Sprintf("  %-12s  %-8s  %-14s  %-10s  %s\n",
		"Period", "Calls", "Tokens saved", "$ saved", "Models"))
	b.WriteString(dash + "\n")
	for _, bk := range series {
		modelMix := ""
		if byModel && len(bk.ByModel) > 0 {
			parts := make([]string, 0, len(bk.ByModel))
			for m, n := range bk.ByModel {
				parts = append(parts, fmt.Sprintf("%s:%d", shortModel(m), n))
			}
			sort.Strings(parts)
			modelMix = strings.Join(parts, " ")
		}
		b.WriteString(fmt.Sprintf("  %-12s  %-8d  %-14d  $%-9.4f  %s\n",
			bk.Period, bk.Commands, bk.TokensSaved, bk.USDSaved, modelMix))
	}
	b.WriteString(dash + "\n")
	return b.String()
}

// shortModel collapses a verbose model id into something compact for the
// table-header byline (e.g. "claude-opus-4-7" → "opus-4-7").
func shortModel(m string) string {
	return strings.TrimPrefix(m, "claude-")
}

func shortSessionID(id string) string {
	if len(id) > 12 {
		return id[:8] + "…"
	}
	return id
}

func progressBar(pct float64, width int) string {
	filled := int(pct / 100 * float64(width))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func humanBytes(n int) string {
	switch {
	case n >= 1024*1024:
		return fmt.Sprintf("%.1fMB", float64(n)/1024/1024)
	case n >= 1024:
		return fmt.Sprintf("%.1fKB", float64(n)/1024)
	default:
		return fmt.Sprintf("%dB", n)
	}
}
