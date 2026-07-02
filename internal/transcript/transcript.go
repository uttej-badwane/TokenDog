// Package transcript reads Claude Code's session transcript JSONL files and
// extracts Anthropic-reported token usage. The on-disk format is undocumented
// and reverse-engineered from real transcripts, with a few field-naming quirks
// (sessionId is camelCase, not snake_case).
//
// Deduplication is streaming-aware: when stop_reason is present, count only
// finalized rows plus the latest unfinished one; otherwise count every row.
// Without this, mid-stream partials get double-counted.
package transcript

import (
	"bufio"
	"encoding/json"
	"os"
	"time"
)

// SessionTotals is the aggregate per-session token usage from one transcript
// file. Numbers come straight from Anthropic's `usage` blocks — these are
// the ground truth that calibration measures cl100k against.
type SessionTotals struct {
	SessionID           string
	InputTokens         int // sum of usage.input_tokens (uncached new input)
	OutputTokens        int // sum of usage.output_tokens
	CacheCreationTokens int // sum of usage.cache_creation_input_tokens
	CacheReadTokens     int // sum of usage.cache_read_input_tokens
	TotalConsumedTokens int // input + output + cache_creation + cache_read
	TotalContextTokens  int // input + cache_creation + cache_read (no output)
	LastTimestamp       time.Time
	NumAPICalls         int
	Path                string
	PredominantModel    string         // model with the most API calls in this session
	ModelCounts         map[string]int // per-model API call count (for sessions that switch)
}

// Entry is a single deduped usage row from a transcript, with its timestamp
// and model preserved. Read collapses these into per-session totals; spend
// reporting needs them ungrouped so it can bucket cost by calendar day.
type Entry struct {
	Timestamp     time.Time
	Model         string
	Input         int // usage.input_tokens (uncached new input)
	Output        int // usage.output_tokens
	CacheCreation int // usage.cache_creation_input_tokens
	CacheRead     int // usage.cache_read_input_tokens
}

// line is the subset of a transcript record we parse. Fields we don't read
// (parentUuid, requestId, etc.) are ignored by encoding/json.
type line struct {
	Type              string `json:"type"`
	SessionID         string `json:"sessionId"`
	Timestamp         string `json:"timestamp"`
	IsSidechain       bool   `json:"isSidechain"`
	IsAPIErrorMessage bool   `json:"isApiErrorMessage"`
	Message           *struct {
		Model      string  `json:"model"`
		StopReason *string `json:"stop_reason"`
		Usage      *struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// readCounted parses one transcript file and applies the streaming-aware
// dedup rule, returning the rows that should be counted. Shared by Read and
// Entries so the dedup logic lives in exactly one place. On per-line parse
// errors the line is skipped (transcripts can contain permission-mode and
// other control rows that don't have `message`).
func readCounted(path string) ([]line, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)

	var entries []line
	hasStopReason := false
	for scanner.Scan() {
		var l line
		if err := json.Unmarshal(scanner.Bytes(), &l); err != nil {
			continue
		}
		if l.Message == nil || l.Message.Usage == nil {
			continue
		}
		entries = append(entries, l)
		if l.Message.StopReason != nil {
			hasStopReason = true
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Streaming-aware dedup: keep finalized entries (stop_reason != null) +
	// the single most recent unfinished entry. Older transcripts without
	// stop_reason keep every row.
	if !hasStopReason {
		return entries, nil
	}
	counted := make([]line, 0, len(entries))
	for i, e := range entries {
		if e.Message.StopReason != nil && *e.Message.StopReason != "" {
			counted = append(counted, e)
		} else if e.Message.StopReason == nil && i == len(entries)-1 {
			counted = append(counted, e)
		}
	}
	return counted, nil
}

// Entries returns the deduped usage rows from a transcript, each tagged with
// its timestamp and model. Rows with an unparseable timestamp keep a zero
// time (they still count toward lifetime totals, just not toward a day).
func Entries(path string) ([]Entry, error) {
	counted, err := readCounted(path)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(counted))
	for _, e := range counted {
		u := e.Message.Usage
		entry := Entry{
			Model:         e.Message.Model,
			Input:         u.InputTokens,
			Output:        u.OutputTokens,
			CacheCreation: u.CacheCreationInputTokens,
			CacheRead:     u.CacheReadInputTokens,
		}
		if e.Timestamp != "" {
			if ts, err := time.Parse(time.RFC3339Nano, e.Timestamp); err == nil {
				entry.Timestamp = ts
			}
		}
		out = append(out, entry)
	}
	return out, nil
}

// Read parses one transcript file and returns the aggregate session totals.
func Read(path string) (*SessionTotals, error) {
	counted, err := readCounted(path)
	if err != nil {
		return nil, err
	}

	t := &SessionTotals{Path: path, ModelCounts: map[string]int{}}
	for _, e := range counted {
		u := e.Message.Usage
		t.InputTokens += u.InputTokens
		t.OutputTokens += u.OutputTokens
		t.CacheCreationTokens += u.CacheCreationInputTokens
		t.CacheReadTokens += u.CacheReadInputTokens
		t.NumAPICalls++
		if t.SessionID == "" && e.SessionID != "" {
			t.SessionID = e.SessionID
		}
		if e.Message.Model != "" {
			t.ModelCounts[e.Message.Model]++
		}
		if !e.IsSidechain && !e.IsAPIErrorMessage && e.Timestamp != "" {
			if ts, err := time.Parse(time.RFC3339Nano, e.Timestamp); err == nil {
				if ts.After(t.LastTimestamp) {
					t.LastTimestamp = ts
				}
			}
		}
	}
	t.TotalConsumedTokens = t.InputTokens + t.OutputTokens + t.CacheCreationTokens + t.CacheReadTokens
	t.TotalContextTokens = t.InputTokens + t.CacheCreationTokens + t.CacheReadTokens
	t.PredominantModel = predominant(t.ModelCounts)
	return t, nil
}

// predominant returns the model with the highest API-call count, or "" if
// the map is empty. Tie-breaks by lex order so output is deterministic.
func predominant(counts map[string]int) string {
	best := ""
	bestN := 0
	for m, n := range counts {
		if n > bestN || (n == bestN && m < best) {
			best = m
			bestN = n
		}
	}
	return best
}
