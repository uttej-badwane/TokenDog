package analytics

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"tokendog/internal/tokenizer"
	"tokendog/internal/transcript"
)

// Calibrator is the minimal interface RenderGain needs to apply calibration
// without importing the calibration package (which would be a cycle —
// calibration depends on analytics for the Record type). Pass nil to render
// uncalibrated.
type Calibrator interface {
	Apply(usd float64) float64
	Confident() bool
	EffectiveRatio() float64
	NumSamples() int
}

// Per-million-token pricing for Anthropic models (input tier). Update when
// pricing changes — used for the USD column in `td gain`. Output tokens are
// not relevant: tool output is fed back to the model as input.
const (
	priceOpus47PerM    = 15.0 // $/M input tokens, standard 200K context
	priceSonnet46PerM  = 3.0
	priceHaiku45PerM   = 0.80
	priceOpus471MPerM  = 30.0 // 1M context premium tier (>200K input)
	defaultModelPerM   = priceOpus47PerM
	defaultModelLabel  = "Opus 4.7"
)

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
	return json.NewEncoder(f).Encode(r)
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
}

func (s Summary) BytesSaved() int  { return s.TotalRawBytes - s.TotalFilteredBytes }
func (s Summary) TokensSaved() int { return s.TotalTokensSaved }

// USDSaved returns dollar savings at the given $/M input-token price. The
// caller picks the model price (Opus, Sonnet, etc.).
func (s Summary) USDSaved(pricePerM float64) float64 {
	return float64(s.TokensSaved()) / 1_000_000 * pricePerM
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
		s.TotalTokensSaved += r.TokensSaved()
		s.TotalDurationMs += r.DurationMs
		if r.CacheHit {
			s.CacheHits++
		}

		name := normalizeName(r.Command)
		cs, ok := byCmd[name]
		if !ok {
			cs = &CommandStat{Name: name}
			byCmd[name] = cs
		}
		cs.Count++
		cs.Saved += r.BytesSaved()
		cs.TokensSaved += r.TokensSaved()
		cs.AvgPct = (cs.AvgPct*float64(cs.Count-1) + r.SavedPct()) / float64(cs.Count)
		cs.AvgMs = (cs.AvgMs*int64(cs.Count-1) + r.DurationMs) / int64(cs.Count)
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

func RenderGain(records []Record, showHistory bool, cal Calibrator) string {
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

	// Cost line — uses real cl100k token counts when available, falls back
	// to bytes/4 otherwise. Defaults to Opus 4.7 standard pricing; users on
	// other models can do their own math from the token count. When the
	// calibrator has enough samples, the USD figure is multiplied by the
	// observed Anthropic-vs-cl100k ratio; otherwise the raw cl100k figure
	// is shown.
	baseUSD := summary.USDSaved(defaultModelPerM)
	tokenSrc := "estimated"
	if tokenizer.Available() && summary.TotalRawTokens > 0 {
		tokenSrc = "cl100k"
	}
	if cal != nil && cal.Confident() {
		calibratedUSD := cal.Apply(baseUSD)
		b.WriteString(fmt.Sprintf("%-22s $%.4f at %s (calibrated %.2f× from %d sessions)\n",
			"Cost saved:", calibratedUSD, defaultModelLabel, cal.EffectiveRatio(), cal.NumSamples()))
	} else {
		samplesNote := ""
		if cal != nil && cal.NumSamples() > 0 {
			samplesNote = fmt.Sprintf(", %d/3 calibration samples", cal.NumSamples())
		}
		b.WriteString(fmt.Sprintf("%-22s $%.4f at %s ($%.0f/M, tokens via %s%s)\n",
			"Cost saved:", baseUSD, defaultModelLabel, defaultModelPerM, tokenSrc, samplesNote))
	}

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

	// USD this session — base is cl100k. We don't apply lifetime calibration
	// here because per-session ratios are too noisy to be useful inline.
	usd := summary.USDSaved(defaultModelPerM)
	b.WriteString(fmt.Sprintf("%-26s $%.4f at %s ($%.0f/M, cl100k)\n",
		"Estimated cost saved:", usd, defaultModelLabel, defaultModelPerM))

	return b.String()
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
