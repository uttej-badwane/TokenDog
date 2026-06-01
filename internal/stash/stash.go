// Package stash implements a content-addressed store for original tool
// outputs that the proxy has compacted lossily. It is the durable half of
// TokenDog's reversible-compression path:
//
//	proxy stashes the full original  → stash.Put → short id
//	proxy injects a compact preview  → "[td:STASHED id=… ]"
//	model calls the td_retrieve MCP tool → stash.Get → full original
//
// Unlike the lossless filters, the reversible path may elide content the
// model can no longer reconstruct from the wire. The contract that keeps it
// honest is that nothing is *lost* — only deferred. As long as the original
// is in the stash, the model can pull it back on demand, so an aggressive
// preview never costs correctness, only an extra round-trip when the middle
// of a large output actually mattered.
//
// The store is plain files under ~/.config/tokendog/originals/, the same
// home dir the proxy daemon and the `td mcp` process both run as. Writes are
// idempotent and content-addressed: identical output reuses the same id,
// so repeated large results dedupe for free.
package stash

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// idLen is how many hex chars of the content sha we expose as the id.
	// 12 chars = 48 bits, collision-safe for the volume of tool outputs in
	// any single session while staying short in the marker the model reads.
	idLen = 12

	envDisable = "TD_REVERSIBLE" // also the master on/off switch (see Enabled)
	envTTL     = "TD_STASH_TTL"  // retention in seconds; default below
	envMinSize = "TD_STASH_MIN"  // min raw bytes before a result is stashable

	defaultTTL     = 24 * time.Hour // long enough to span a working session
	defaultMinSize = 2048           // don't bother stashing small outputs
)

// Record is the on-disk stash entry.
type Record struct {
	ID        string    `json:"id"`
	Command   string    `json:"command"`
	CreatedAt time.Time `json:"created_at"`
	OrigBytes int       `json:"orig_bytes"`
	Content   string    `json:"content"`
}

// Enabled reports whether reversible compression is turned on. It is opt-in:
// the feature changes TokenDog's default lossless behavior, so a user must
// set TD_REVERSIBLE=1 to accept the trade (aggressive preview now, retrieval
// round-trip if the elided middle is needed).
func Enabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(envDisable))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// MinSize is the smallest raw output (in bytes) worth stashing. Below this,
// the preview marker would cost more than it saves, so the caller should
// leave small results untouched.
func MinSize() int {
	if s := os.Getenv(envMinSize); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return n
		}
	}
	return defaultMinSize
}

// TTL returns how long stashed originals are retained before pruning.
func TTL() time.Duration {
	if s := os.Getenv(envTTL); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return defaultTTL
}

func dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	d := filepath.Join(home, ".config", "tokendog", "originals")
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", err
	}
	return d, nil
}

// ID derives the content-addressed id for a piece of content.
func ID(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])[:idLen]
}

// Put stores content and returns its id. Writes are idempotent: the same
// content always yields the same id, and re-Putting refreshes the entry's
// mtime so the TTL window slides on reuse. Best-effort — a write failure
// returns the id with the error so the caller can decide whether to fall
// back to the un-stashed original.
func Put(command, content string) (string, error) {
	id := ID(content)
	d, err := dir()
	if err != nil {
		return id, err
	}
	rec := Record{
		ID:        id,
		Command:   command,
		CreatedAt: time.Now(),
		OrigBytes: len(content),
		Content:   content,
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return id, err
	}
	path := filepath.Join(d, id+".json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return id, err
	}
	go prune(d)
	return id, nil
}

// Get returns the stashed record for id. Missing, malformed, or expired
// entries return (nil, false). Retrieval does NOT enforce the TTL strictly —
// if the file still exists we serve it, because a model asking for an
// original is a strong signal it is still relevant; pruning is the only
// reaper.
func Get(id string) (*Record, bool) {
	d, err := dir()
	if err != nil {
		return nil, false
	}
	data, err := os.ReadFile(filepath.Join(d, id+".json"))
	if err != nil {
		return nil, false
	}
	var rec Record
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, false
	}
	return &rec, true
}

// List returns every live stash record, newest first. Used by `td stash`.
func List() ([]Record, error) {
	d, err := dir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(d)
	if err != nil {
		return nil, err
	}
	var recs []Record
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(d, ent.Name()))
		if err != nil {
			continue
		}
		var rec Record
		if err := json.Unmarshal(data, &rec); err != nil {
			continue
		}
		recs = append(recs, rec)
	}
	// Newest first — simple insertion sort, the set is small.
	for i := 1; i < len(recs); i++ {
		for j := i; j > 0 && recs[j].CreatedAt.After(recs[j-1].CreatedAt); j-- {
			recs[j], recs[j-1] = recs[j-1], recs[j]
		}
	}
	return recs, nil
}

// Purge deletes every stashed original and returns how many were removed.
func Purge() (int, error) {
	d, err := dir()
	if err != nil {
		return 0, err
	}
	entries, err := os.ReadDir(d)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".json") {
			continue
		}
		if os.Remove(filepath.Join(d, ent.Name())) == nil {
			n++
		}
	}
	return n, nil
}

// prune removes entries older than the TTL. Best-effort and bounded to the
// originals dir; ignores errors.
func prune(d string) {
	entries, err := os.ReadDir(d)
	if err != nil {
		return
	}
	ttl := TTL()
	now := time.Now()
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		info, err := ent.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > ttl {
			_ = os.Remove(filepath.Join(d, ent.Name()))
		}
	}
}

// Preview builds the compact lossy stand-in the proxy injects in place of a
// stashed original: the first headLines and last tailLines joined by an
// elision marker that names the id and how to recover the full text. It is
// content-agnostic on purpose — head+tail is what an agent scanning a large
// log, file read, or JSON dump usually needs, and the retrieval tool covers
// the rest. Returns the original unchanged if it is already short enough that
// eliding wouldn't help.
func Preview(id, content string, headLines, tailLines int) string {
	lines := strings.Split(content, "\n")
	if len(lines) <= headLines+tailLines+1 {
		return content
	}
	head := lines[:headLines]
	tail := lines[len(lines)-tailLines:]
	elided := len(lines) - headLines - tailLines

	var b strings.Builder
	b.WriteString(strings.Join(head, "\n"))
	b.WriteString("\n")
	b.WriteString(marker(id, elided, len(content)))
	b.WriteString("\n")
	b.WriteString(strings.Join(tail, "\n"))
	return b.String()
}

// marker is the single elision line the model reads. It states the id, the
// scale of what was elided, and exactly how to get it back, so the model can
// decide whether a retrieve round-trip is worth it.
func marker(id string, elidedLines, origBytes int) string {
	return "[td:STASHED id=" + id +
		" — " + strconv.Itoa(elidedLines) + " lines / " + humanBytes(origBytes) +
		" elided. Call the td_retrieve tool (tokendog MCP server) with id=\"" + id +
		"\" to get the full original output.]"
}

func humanBytes(n int) string {
	switch {
	case n >= 1<<20:
		return strconv.FormatFloat(float64(n)/(1<<20), 'f', 1, 64) + "MB"
	case n >= 1<<10:
		return strconv.FormatFloat(float64(n)/(1<<10), 'f', 1, 64) + "KB"
	default:
		return strconv.Itoa(n) + "B"
	}
}
