package stash

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Retrieval telemetry. Every time the model pulls a stashed original back via
// the td_retrieve MCP tool, we append one line here. The signal is the whole
// point of `td learn`: a command whose previews get retrieved often is one
// whose head/tail elision is too aggressive — the model keeps needing the
// middle. A command that is stashed but never retrieved is a clean win.
//
// Stored as JSONL alongside the originals so it survives across sessions and
// is trivially greppable. Logging is best-effort: a failure to record
// telemetry must never break a retrieval.

const retrievalsFile = "retrievals.jsonl"

// Retrieval is one logged td_retrieve call.
type Retrieval struct {
	ID      string    `json:"id"`
	Command string    `json:"command"`
	At      time.Time `json:"at"`
}

func retrievalsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	d := filepath.Join(home, ".config", "tokendog")
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(d, retrievalsFile), nil
}

// LogRetrieval appends a retrieval record. Best-effort: errors are swallowed
// because telemetry is never worth failing a retrieval over.
func LogRetrieval(id, command string) {
	path, err := retrievalsPath()
	if err != nil {
		return
	}
	rec := Retrieval{ID: id, Command: command, At: time.Now()}
	data, err := json.Marshal(rec)
	if err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(data, '\n'))
}

// LoadRetrievals reads every logged retrieval. A missing file is not an error
// (it just means nothing has been retrieved yet); malformed lines are skipped.
func LoadRetrievals() ([]Retrieval, error) {
	path, err := retrievalsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Retrieval
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var r Retrieval
		if err := json.Unmarshal(line, &r); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// splitLines splits on '\n' without allocating a big intermediate string,
// returning byte slices that reference the input.
func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
