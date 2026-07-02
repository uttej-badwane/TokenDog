// Package sessioncost persists the cost figures Claude Code itself computes and
// pushes to a statusLine command on stdin (the cost.total_cost_usd field). This
// is the same number `/cost` and `/usage` show, so surfacing it sidesteps
// TokenDog's own token-pricing entirely and can never disagree with Claude
// Code's estimate.
//
// The store is an append-only JSONL log: the statusline shim appends one line
// per render (fast, lock-free, never loses data under concurrent Claude Code
// windows), and readers collapse it to the latest sample per session id.
// cost.total_cost_usd is cumulative for the life of a session, so "latest wins"
// per session yields that session's final cost, and summing across sessions
// gives the period total.
package sessioncost

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Sample is one observation of a session's cumulative cost, as reported by
// Claude Code's statusLine stdin payload.
type Sample struct {
	SessionID string    `json:"session_id"`
	Model     string    `json:"model,omitempty"`
	CostUSD   float64   `json:"cost_usd"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Path returns the store location. TD_SESSION_COSTS overrides it (tests,
// non-standard installs); otherwise it sits with the rest of TokenDog's state
// under ~/.config/tokendog. Empty string means the home dir is unresolvable.
func Path() string {
	if p := os.Getenv("TD_SESSION_COSTS"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "tokendog", "session-costs.jsonl")
}

// Append records one sample as a single JSON line. It creates the parent
// directory on first use and opens with O_APPEND so concurrent writers from
// multiple Claude Code windows interleave whole lines rather than clobbering
// each other. Callers treat errors as non-fatal — a dropped sample is corrected
// by the next render, and the statusline must never fail on our account.
func Append(s Sample) error {
	path := Path()
	if path == "" {
		return os.ErrInvalid
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	line, err := json.Marshal(s)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return err
}

// Load returns the latest sample per session id. A missing store is not an
// error — it just means no statusline capture has happened yet, so callers fall
// back to token-priced spend. Malformed lines are skipped.
func Load() (map[string]Sample, error) {
	path := Path()
	if path == "" {
		return map[string]Sample{}, nil
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]Sample{}, nil
		}
		return nil, err
	}
	defer f.Close()

	latest := map[string]Sample{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var s Sample
		if err := json.Unmarshal(sc.Bytes(), &s); err != nil || s.SessionID == "" {
			continue
		}
		// Cost is cumulative per session, so the highest sample is that
		// session's final cost. Preferring max cost (tie-broken by newer
		// timestamp) is robust to the documented /resume case where a later
		// sample can under-report.
		if prev, ok := latest[s.SessionID]; ok {
			if s.CostUSD < prev.CostUSD ||
				(s.CostUSD == prev.CostUSD && s.UpdatedAt.Before(prev.UpdatedAt)) {
				continue
			}
		}
		latest[s.SessionID] = s
	}
	return latest, sc.Err()
}

// Compact rewrites the store keeping only the latest sample per session id,
// bounding the append-only log's growth. It is best-effort: on any error the
// original file is left intact. Safe to call opportunistically from readers.
func Compact() error {
	latest, err := Load()
	if err != nil || len(latest) == 0 {
		return err
	}
	path := Path()
	tmp, err := os.CreateTemp(filepath.Dir(path), ".session-costs-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename

	w := bufio.NewWriter(tmp)
	enc := json.NewEncoder(w)
	for _, s := range latest {
		if err := enc.Encode(s); err != nil {
			tmp.Close()
			return err
		}
	}
	if err := w.Flush(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
