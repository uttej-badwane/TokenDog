// Package replay walks Claude Code transcript JSONLs and replays each Bash
// tool_use through a caller-provided filter to compute counterfactual
// savings: "how much would TD have saved you on these sessions if it had
// been active at the time?"
//
// The transcript Bash tool_result content is what Claude actually saw — so
// we have ground-truth-ish input for each historical command. After running
// it through TD's current filters, we tokenize raw + filtered with cl100k
// and accumulate.
//
// Caveats — surface them in the renderer, not silently:
//   - Filter behavior evolves. Replaying old outputs through *current* filters
//     projects savings at today's filter quality, not historical quality.
//   - Tool_result content is what Claude saw, not raw command output. Claude
//     Code applies its own truncation, so very large outputs are already
//     capped — replay underestimates savings on those.
//   - Some Bash calls weren't filterable by TD even at runtime (echo, custom
//     scripts) — those are reported as "not handled" and excluded from the
//     would-have-saved totals.
package replay

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"tokendog/internal/tokenizer"
)

// DispatchFn classifies a command, runs the appropriate filter on the raw
// output, and returns the filtered result plus classification info.
// cmd/replay.go provides this so dispatch lives in cmd-layer (with access
// to subcmd helpers and the filter package) while internal/replay focuses
// on transcript walking.
//
// Two distinct outcomes:
//   - info.Handled=true means the binary was in Supported and the filter
//     ran. The filter may have produced a no-op (filtered == raw) for
//     legitimate reasons (output was already minimal); that still counts
//     as handled, just with zero savings.
//   - info.Handled=false means the binary wasn't recognized — TD has no
//     filter for it. These accumulate in UnhandledTopN.
type DispatchFn func(command, raw string) (filtered string, info DispatchInfo)

// DispatchInfo describes how a command was classified by the dispatcher.
type DispatchInfo struct {
	Handled bool   // true when binary is in hook.Supported
	Binary  string // canonical binary name ("git", "aws", etc.)
	Subcmd  string // first positional non-flag, value-flag-aware arg
}

// Key returns the per-command grouping key used in PerCommand stats.
func (d DispatchInfo) Key() string {
	if d.Subcmd != "" {
		return d.Binary + " " + d.Subcmd
	}
	return d.Binary
}

// Result is the aggregate of a replay across many transcripts.
type Result struct {
	SessionsScanned  int
	BashCallsSeen    int
	BashCallsHandled int // how many had a Supported binary
	RawBytes         int
	FilteredBytes    int
	RawTokens        int
	FilteredTokens   int
	PerCommand       map[string]*CommandStat
	PerSession       []*SessionStat
	UnhandledTopN    map[string]int // binary → count for "not handled" report
}

// CommandStat aggregates by command (e.g. "gh run" or "aws ec2").
type CommandStat struct {
	Name           string
	Calls          int
	RawBytes       int
	FilteredBytes  int
	RawTokens      int
	FilteredTokens int
}

// SessionStat is one transcript file's contribution.
type SessionStat struct {
	SessionID    string
	Path         string
	LastActivity time.Time
	BashCalls    int
	RawTokens    int
	TokensSaved  int
}

// Options controls walk behavior. Zero value walks everything.
type Options struct {
	// Since limits to transcripts modified after this time. Zero = no limit.
	Since time.Time
	// MaxSessions caps the number of sessions to scan (after sorting newest-
	// first). Zero = no cap.
	MaxSessions int
}

// transcriptLine is a single JSONL row. We only decode the parts we need.
type transcriptLine struct {
	Type      string `json:"type"`
	SessionID string `json:"sessionId"`
	Timestamp string `json:"timestamp"`
	Message   *struct {
		Content json.RawMessage `json:"content"`
		Role    string          `json:"role"`
	} `json:"message"`
}

// contentBlock covers both tool_use (in assistant messages) and tool_result
// (in user messages). The shapes overlap enough for one struct.
type contentBlock struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     toolUseInput    `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
}

type toolUseInput struct {
	Command string `json:"command"`
}

// Walk scans rootDir for transcript JSONLs and replays each through dispatch.
// Returns the aggregate Result. Errors on individual files are skipped so a
// single corrupt transcript doesn't abort the whole replay.
func Walk(rootDir string, dispatch DispatchFn, opts Options) (*Result, error) {
	files, err := findTranscripts(rootDir, opts)
	if err != nil {
		return nil, err
	}

	r := &Result{
		PerCommand:    map[string]*CommandStat{},
		UnhandledTopN: map[string]int{},
	}

	for _, path := range files {
		s, err := replayFile(path, dispatch, r)
		if err != nil {
			continue
		}
		if s != nil {
			r.PerSession = append(r.PerSession, s)
			r.SessionsScanned++
		}
	}
	return r, nil
}

type transcriptEntry struct {
	path  string
	mtime time.Time
}

// findTranscripts walks rootDir for *.jsonl files, applying Since and
// MaxSessions filters. Returns paths sorted newest-first by mtime so
// MaxSessions trims the oldest sessions.
func findTranscripts(rootDir string, opts Options) ([]string, error) {
	var entries []transcriptEntry

	err := filepath.WalkDir(rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if !opts.Since.IsZero() && info.ModTime().Before(opts.Since) {
			return nil
		}
		entries = append(entries, transcriptEntry{path: path, mtime: info.ModTime()})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].mtime.After(entries[j].mtime)
	})
	if opts.MaxSessions > 0 && len(entries) > opts.MaxSessions {
		entries = entries[:opts.MaxSessions]
	}
	paths := make([]string, len(entries))
	for i, e := range entries {
		paths[i] = e.path
	}
	return paths, nil
}

// replayFile parses one transcript and accumulates into r. Returns the
// per-session summary or nil if the file had no Bash activity.
func replayFile(path string, dispatch DispatchFn, r *Result) (*SessionStat, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 64*1024*1024)

	type pending struct {
		command string
	}
	open := map[string]pending{} // tool_use_id → command
	s := &SessionStat{Path: path}

	for scanner.Scan() {
		var line transcriptLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}
		if s.SessionID == "" && line.SessionID != "" {
			s.SessionID = line.SessionID
		}
		if line.Timestamp != "" {
			if ts, err := time.Parse(time.RFC3339Nano, line.Timestamp); err == nil {
				if ts.After(s.LastActivity) {
					s.LastActivity = ts
				}
			}
		}
		if line.Message == nil || len(line.Message.Content) == 0 {
			continue
		}

		// Content can be a string OR an array of blocks. We only care about
		// the array case — Bash tool_use/tool_result blocks live there.
		var blocks []contentBlock
		if err := json.Unmarshal(line.Message.Content, &blocks); err != nil {
			continue
		}

		for _, b := range blocks {
			switch b.Type {
			case "tool_use":
				if b.Name == "Bash" && b.ID != "" && b.Input.Command != "" {
					open[b.ID] = pending{command: b.Input.Command}
				}
			case "tool_result":
				p, ok := open[b.ToolUseID]
				if !ok {
					continue
				}
				delete(open, b.ToolUseID)
				raw := extractResultText(b.Content)
				if raw == "" {
					continue
				}
				accumulate(r, s, p.command, raw, dispatch)
			}
		}
	}
	if s.BashCalls == 0 {
		return nil, nil
	}
	return s, nil
}

// extractResultText handles both shapes Anthropic uses for tool_result
// content: a bare string or an array of {type: "text", text: "..."} blocks.
// Returns the concatenated text, empty string on failure.
func extractResultText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// Try string first.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	// Then array of text blocks.
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var b strings.Builder
		for _, blk := range blocks {
			if blk.Type == "text" {
				b.WriteString(blk.Text)
			}
		}
		return b.String()
	}
	return ""
}

// accumulate runs the filter and updates per-command + per-session totals.
func accumulate(r *Result, s *SessionStat, command, raw string, dispatch DispatchFn) {
	r.BashCallsSeen++
	s.BashCalls++

	filtered, info := dispatch(command, raw)
	if !info.Handled {
		if bin := firstWord(command); bin != "" {
			r.UnhandledTopN[bin]++
		}
		return
	}
	r.BashCallsHandled++

	rawTok := tokenizer.Count(raw)
	filtTok := tokenizer.Count(filtered)
	r.RawBytes += len(raw)
	r.FilteredBytes += len(filtered)
	r.RawTokens += rawTok
	r.FilteredTokens += filtTok
	s.RawTokens += rawTok
	s.TokensSaved += rawTok - filtTok

	name := info.Key()
	cs, ok := r.PerCommand[name]
	if !ok {
		cs = &CommandStat{Name: name}
		r.PerCommand[name] = cs
	}
	cs.Calls++
	cs.RawBytes += len(raw)
	cs.FilteredBytes += len(filtered)
	cs.RawTokens += rawTok
	cs.FilteredTokens += filtTok
}

// firstWord extracts the leading binary name from a compound command for
// the unhandled-binaries report. Strips path prefixes; returns empty for
// empty input.
func firstWord(s string) string {
	parts := strings.Fields(s)
	idx := 0
	// Skip env-var prefixes (but not "key=" arguments).
	for idx < len(parts) {
		p := parts[idx]
		eq := strings.IndexByte(p, '=')
		if eq <= 0 {
			break
		}
		// Crude env-var check: NAME=VALUE where NAME is uppercase letters,
		// digits, underscores. Same as hook.IsEnvAssignment but inlined to
		// avoid the import.
		ok := true
		for i := 0; i < eq; i++ {
			c := p[i]
			isAlpha := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
			isDigit := c >= '0' && c <= '9'
			if !(isAlpha || isDigit || c == '_') {
				ok = false
				break
			}
		}
		if !ok {
			break
		}
		idx++
	}
	if idx >= len(parts) {
		return ""
	}
	w := parts[idx]
	if i := strings.LastIndex(w, "/"); i >= 0 {
		w = w[i+1:]
	}
	return w
}

// TokensSaved is the headline number for the renderer.
func (r *Result) TokensSaved() int { return r.RawTokens - r.FilteredTokens }

// SavedPct is 0-100 saved fraction across handled calls.
func (r *Result) SavedPct() float64 {
	if r.RawTokens == 0 {
		return 0
	}
	return float64(r.TokensSaved()) / float64(r.RawTokens) * 100
}
