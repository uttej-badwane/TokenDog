// Package calibration cross-references TokenDog's cl100k token estimates
// against Anthropic's reported usage from transcript JSONL files. The output
// is a rolling ratio applied to td gain's USD calculation so users see an
// empirically-backed cost figure instead of a pure cl100k estimate.
//
// Caveats (call them out where this ratio is shown):
//
//   - The ratio is total-session: numerator includes Anthropic's full context
//     consumption (system prompt + user messages + tool results), denominator
//     is only the cl100k tokens of TD-filtered tool output. So the value is
//     biased high by whatever fraction of input was NOT tool output.
//   - Over many sessions in a tool-heavy workflow, the bias stabilizes and
//     the ratio becomes a useful directional signal — but it is not a pure
//     cl100k-vs-Anthropic tokenizer calibration.
//   - We sample-weight the average so larger sessions dominate (their ratios
//     are less noisy).
//
// Stored at ~/.config/tokendog/calibration.json; recomputed on demand by
// td gain when invoked. Data flow:
//
//	transcript JSONL → Anthropic-counted input/cache tokens (numerator)
//	td analytics history → cl100k-counted RawTokens for same session_id (denom)
//	ratio per session → weighted average across N most recent sessions → applied
package calibration

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"tokendog/internal/analytics"
	"tokendog/internal/transcript"
)

const (
	// MaxSessions caps the rolling window. Older sessions don't add precision
	// — they just dilute the signal if the user's workflow shifted.
	MaxSessions = 20
	// MinSamples is the count below which we don't apply calibration at all.
	// One or two sessions is too noisy; users see uncalibrated cl100k numbers
	// until the signal accumulates.
	MinSamples = 3
)

// Snapshot is the persisted state. Stored as ~/.config/tokendog/calibration.json.
type Snapshot struct {
	Sessions     []SessionSample `json:"sessions"`
	Ratio        float64         `json:"ratio"`
	LastComputed time.Time       `json:"last_computed"`
}

// SessionSample is one session's contribution to the rolling average.
type SessionSample struct {
	SessionID         string  `json:"session_id"`
	AnthropicInput    int     `json:"anthropic_input"` // input + cache_creation (newly-seen content)
	CL100KFromTD      int     `json:"cl100k_from_td"`  // sum of RawTokens for records in this session
	Ratio             float64 `json:"ratio"`
	NumTDRecords      int     `json:"num_td_records"`
	ObservedAt        time.Time `json:"observed_at"`
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
	return filepath.Join(dir, "calibration.json"), nil
}

// Load reads the persisted snapshot. Missing file returns a zero Snapshot
// with no error — callers treat that as "no calibration yet, use 1.0".
func Load() (*Snapshot, error) {
	path, err := dataPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Snapshot{}, nil
	}
	if err != nil {
		return nil, err
	}
	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		// Treat corrupt state as fresh — calibration is best-effort.
		return &Snapshot{}, nil
	}
	return &s, nil
}

// Save persists the snapshot. Errors are returned but callers usually log
// and continue — analytics output should never depend on calibration write.
func Save(s *Snapshot) error {
	path, err := dataPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Recompute scans recent records and their transcripts to update the rolling
// ratio. It groups records by SessionID, reads the transcript at each
// session's TranscriptPath, and recomputes per-session ratios.
//
// Records without a SessionID (legacy or env-injection-disabled) are
// skipped silently — they can't be matched to a transcript.
func Recompute(records []analytics.Record) (*Snapshot, error) {
	type bucket struct {
		sessionID      string
		transcriptPath string
		cl100kRaw      int
		numRecords     int
		latestTS       time.Time
	}
	buckets := map[string]*bucket{}
	for _, r := range records {
		if r.SessionID == "" {
			continue
		}
		b, ok := buckets[r.SessionID]
		if !ok {
			b = &bucket{sessionID: r.SessionID, transcriptPath: r.TranscriptPath}
			buckets[r.SessionID] = b
		}
		// Prefer a non-empty transcript path if any record has one — older
		// records in the same session may pre-date the env-injection.
		if b.transcriptPath == "" && r.TranscriptPath != "" {
			b.transcriptPath = r.TranscriptPath
		}
		b.cl100kRaw += r.RawTokens
		b.numRecords++
		if r.Timestamp.After(b.latestTS) {
			b.latestTS = r.Timestamp
		}
	}

	var samples []SessionSample
	for _, b := range buckets {
		if b.transcriptPath == "" || b.cl100kRaw == 0 {
			continue
		}
		t, err := transcript.Read(b.transcriptPath)
		if err != nil {
			continue
		}
		// Numerator: newly-seen content (input + cache_creation). Cache_read
		// is excluded because it represents *re-reads* of content the model
		// already has cached — it's not new tokens in the same sense as the
		// fresh tool output that TD filters.
		anth := t.InputTokens + t.CacheCreationTokens
		if anth == 0 {
			continue
		}
		samples = append(samples, SessionSample{
			SessionID:      b.sessionID,
			AnthropicInput: anth,
			CL100KFromTD:   b.cl100kRaw,
			Ratio:          float64(anth) / float64(b.cl100kRaw),
			NumTDRecords:   b.numRecords,
			ObservedAt:     b.latestTS,
		})
	}

	// Keep only the MaxSessions most recent samples.
	sort.Slice(samples, func(i, j int) bool {
		return samples[i].ObservedAt.After(samples[j].ObservedAt)
	})
	if len(samples) > MaxSessions {
		samples = samples[:MaxSessions]
	}

	s := &Snapshot{Sessions: samples, LastComputed: time.Now()}
	s.Ratio = weightedRatio(samples)
	return s, nil
}

// weightedRatio computes sum(anthropic) / sum(cl100k) across all samples.
// This is mathematically equivalent to a sample-weighted average of the
// per-session ratios — sessions with more TD activity contribute more.
func weightedRatio(samples []SessionSample) float64 {
	var anth, cl int
	for _, s := range samples {
		anth += s.AnthropicInput
		cl += s.CL100KFromTD
	}
	if cl == 0 {
		return 0
	}
	return float64(anth) / float64(cl)
}

// Apply returns the calibrated USD for a base USD figure. Below MinSamples
// returns the input unchanged. Apply is intentionally a method on Snapshot
// rather than a free function so callers must explicitly load state — there
// is no implicit "global" calibration.
func (s *Snapshot) Apply(usd float64) float64 {
	if s == nil || len(s.Sessions) < MinSamples || s.Ratio <= 0 {
		return usd
	}
	return usd * s.Ratio
}

// Confident reports whether the snapshot has enough samples to be applied.
// Used by the renderer to decide whether to show "calibrated" vs "estimated".
func (s *Snapshot) Confident() bool {
	return s != nil && len(s.Sessions) >= MinSamples && s.Ratio > 0
}

// NumSamples reports how many sessions have contributed to the rolling
// average. Renderers use this to show a "N/3 samples until calibrated"
// progress hint while the snapshot is still warming up.
func (s *Snapshot) NumSamples() int {
	if s == nil {
		return 0
	}
	return len(s.Sessions)
}

// EffectiveRatio is the method form of Ratio for satisfying analytics.Calibrator.
// We can't name a method the same as the field; the renderer-facing API
// goes through this method while persisted JSON keeps the simpler "ratio".
func (s *Snapshot) EffectiveRatio() float64 {
	if s == nil {
		return 0
	}
	return s.Ratio
}
