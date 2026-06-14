package spend

import (
	"encoding/json"
	"os"
	"path/filepath"

	"tokendog/internal/transcript"
)

// cacheVersion is bumped whenever the cached shape changes, so an older cache
// file is treated as a cold start rather than mis-decoded.
const cacheVersion = 1

// fileEntry is one transcript's cached parse result, validated against the
// file's size and modtime so a changed transcript is re-parsed.
type fileEntry struct {
	ModTimeNano int64              `json:"mtime"`
	Size        int64              `json:"size"`
	Entries     []transcript.Entry `json:"entries"`
}

// cacheData is the on-disk parse cache: a transcript's token rows keyed by
// absolute path. Only the raw token rows are cached, never priced cost, so a
// pricing-data update needs no cache invalidation.
type cacheData struct {
	Version int                  `json:"version"`
	Files   map[string]fileEntry `json:"files"`
}

// entryReader serves transcript rows from the cache when a file is unchanged
// and re-parses (then records) only files whose size or modtime moved. Without
// it, every `td spend` re-parses the whole ~/.claude/projects tree — wasteful
// for the always-running menu-bar agent that re-invokes `td spend` on a timer.
type entryReader struct {
	data  cacheData
	dirty bool
}

// cachePath returns the parse-cache location. TD_SPEND_CACHE overrides it (for
// tests and non-standard installs); otherwise it sits alongside the rest of
// TokenDog's state under ~/.config/tokendog.
func cachePath() string {
	if p := os.Getenv("TD_SPEND_CACHE"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "tokendog", "spend-cache.json")
}

// loadCache reads the parse cache, returning an empty but usable reader on any
// error or version mismatch — a missing or corrupt cache is just a cold start,
// never a failure.
func loadCache() *entryReader {
	r := &entryReader{data: cacheData{Version: cacheVersion, Files: map[string]fileEntry{}}}
	path := cachePath()
	if path == "" {
		return r
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return r
	}
	var d cacheData
	if err := json.Unmarshal(b, &d); err != nil || d.Version != cacheVersion || d.Files == nil {
		return r
	}
	r.data = d
	return r
}

// entries returns a transcript's token rows, from cache when the file's size
// and modtime are unchanged, otherwise by parsing and recording the result.
func (r *entryReader) entries(path string, d os.DirEntry) ([]transcript.Entry, error) {
	info, err := d.Info()
	if err != nil {
		return transcript.Entries(path) // can't validate freshness; parse uncached
	}
	mtime := info.ModTime().UnixNano()
	size := info.Size()
	if fe, ok := r.data.Files[path]; ok && fe.ModTimeNano == mtime && fe.Size == size {
		return fe.Entries, nil
	}
	entries, err := transcript.Entries(path)
	if err != nil {
		return nil, err
	}
	r.data.Files[path] = fileEntry{ModTimeNano: mtime, Size: size, Entries: entries}
	r.dirty = true
	return entries, nil
}

// prune drops cache records for transcripts not present in `seen` (deleted
// sessions), keeping the cache from growing without bound.
func (r *entryReader) prune(seen map[string]struct{}) {
	for path := range r.data.Files {
		if _, ok := seen[path]; !ok {
			delete(r.data.Files, path)
			r.dirty = true
		}
	}
}

// save writes the cache back atomically when anything changed. Best-effort: a
// write failure just means the next run starts colder, never a spend error.
func (r *entryReader) save() {
	if !r.dirty {
		return
	}
	path := cachePath()
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	b, err := json.Marshal(r.data)
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}
