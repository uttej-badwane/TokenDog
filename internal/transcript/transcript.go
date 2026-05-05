// Package transcript reads Claude Code's session transcript JSONL files and
// extracts Anthropic-reported token usage. The format is what's described in
// ccstatusline's reverse-engineering notes, modulo a few field-naming quirks
// in real data (sessionId is camelCase, not snake_case).
//
// We follow ccstatusline's streaming-aware deduplication rule from §3.2 of
// those notes: when stop_reason is present, count only finalized rows plus
// the latest unfinished one; otherwise count every row. Without this, mid-
// stream partials get double-counted.
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
	SessionID            string
	InputTokens          int   // sum of usage.input_tokens (uncached new input)
	OutputTokens         int   // sum of usage.output_tokens
	CacheCreationTokens  int   // sum of usage.cache_creation_input_tokens
	CacheReadTokens      int   // sum of usage.cache_read_input_tokens
	TotalConsumedTokens  int   // input + output + cache_creation + cache_read
	TotalContextTokens   int   // input + cache_creation + cache_read (no output)
	LastTimestamp        time.Time
	NumAPICalls          int
	Path                 string
}

// line mirrors the shape ccstatusline parses with Zod. Fields we don't read
// (parentUuid, requestId, etc.) are ignored by encoding/json.
type line struct {
	Type              string `json:"type"`
	SessionID         string `json:"sessionId"`
	Timestamp         string `json:"timestamp"`
	IsSidechain       bool   `json:"isSidechain"`
	IsAPIErrorMessage bool   `json:"isApiErrorMessage"`
	Message           *struct {
		StopReason *string `json:"stop_reason"`
		Usage      *struct {
			InputTokens             int `json:"input_tokens"`
			OutputTokens            int `json:"output_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// Read parses one transcript file and returns the aggregate session totals.
// On parse errors per-line, the line is skipped (transcripts can contain
// permission-mode and other control rows that don't have `message`).
func Read(path string) (*SessionTotals, error) {
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
	counted := make([]line, 0, len(entries))
	if hasStopReason {
		for i, e := range entries {
			if e.Message.StopReason != nil && *e.Message.StopReason != "" {
				counted = append(counted, e)
			} else if e.Message.StopReason == nil && i == len(entries)-1 {
				counted = append(counted, e)
			}
		}
	} else {
		counted = entries
	}

	t := &SessionTotals{Path: path}
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
	return t, nil
}
