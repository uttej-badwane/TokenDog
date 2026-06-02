// Package core is TokenDog's provider-neutral compression engine. It knows
// nothing about Anthropic, OpenAI, HTTP, or analytics — it operates on a
// Conversation (a flat list of tool results in order) and decides, for the
// results eligible for replacement, whether dedup / per-tool filtering /
// generic compaction / reversible stashing shrinks them.
//
// Provider adapters (internal/adapter/*) translate a wire request into a
// Conversation, run Compress, and write any replacements back into the
// original payload. Frontends (the MITM proxy, the explicit-base_url gateway,
// or an SDK middleware) wire an adapter to a transport. This is the decoupling
// that lets the same engine serve every provider and every deployment shape
// instead of being welded to one MITM'd endpoint.
package core

import (
	"os"
	"strings"

	"tokendog/internal/policy"
	"tokendog/internal/stash"
)

// ToolResult is one compressible unit the engine sees: the text a tool
// produced, plus enough context to compress it. Adapters build these from
// whatever their wire format calls a tool result.
type ToolResult struct {
	// Command is the shell command that produced this output, or "" when the
	// producer isn't a shell command (e.g. a file-read tool). Used to pick a
	// per-tool filter and to label savings; dedup works regardless.
	Command string
	// Text is the current output text. The engine reads this.
	Text string
	// Eligible marks results the engine may replace. Only the newest turn's
	// results are eligible — modifying older ones would invalidate the
	// provider's prompt cache (see the package docs on cache safety).
	Eligible bool

	// The engine sets these when it decides to replace Text. Adapters apply
	// Replacement back into the wire payload only when Replaced is true.
	Replacement string
	Replaced    bool
}

// Conversation is the ordered, provider-neutral view Compress needs.
type Conversation struct {
	Results []*ToolResult
}

// Saving is one recorded compression event, for the frontend to push into
// analytics. Label is the analytics command label WITHOUT the "proxy:"
// prefix the frontend adds, matching the historical schema so `td gain`
// aggregates old and new records identically.
type Saving struct {
	Label    string
	Original string
	Result   string
}

// Options toggles the lossy/opt-in passes. Lossless passes (per-tool filter,
// generic JSON) always run. Kept as plain data so the engine is trivially
// testable; frontends populate it from the environment via OptionsFromEnv.
type Options struct {
	// Dedup enables cross-message back-referencing (lossless, on by default).
	Dedup bool
	// Reversible enables stash+preview of large outputs (opt-in; changes the
	// default lossless behavior, so it is off unless explicitly requested).
	Reversible bool
	// StashMinBytes overrides the reversible-stash size threshold. 0 means
	// "use the stash package default".
	StashMinBytes int
	// Prose, when set, is a lossy natural-language compressor (e.g. an ML
	// model behind a localhost sidecar). The engine uses it ONLY inside the
	// reversible pass to build a denser preview for prose content — the full
	// original is always stashed and recoverable via td_retrieve, so the
	// lossiness never costs correctness, only an optional retrieval. nil =
	// disabled. Frontends inject it (internal/prose) so the engine stays
	// I/O-free.
	Prose ProseFunc
}

// ProseFunc compresses natural-language text, returning (compressed, ok).
// ok=false means "couldn't help — leave it alone". Implementations live in
// frontends (internal/prose), keeping internal/core free of HTTP/network.
type ProseFunc func(text string) (string, bool)

const (
	envNoDedup    = "TD_NO_DEDUP"
	envReversible = "TD_REVERSIBLE"
	envStashMin   = "TD_STASH_MIN"
)

// OptionsFromEnv derives Options with precedence: an explicitly-set
// environment variable (the developer's local override) wins over the managed
// policy file, which wins over the built-in default. This keeps existing
// env-driven behavior unchanged while letting a platform team set a fleet
// baseline via policy.
func OptionsFromEnv() Options {
	pol := policy.Load()

	o := Options{Dedup: true, Reversible: false, StashMinBytes: 0}

	// Managed-policy baseline.
	if pol.Dedup != nil {
		o.Dedup = *pol.Dedup
	}
	if pol.Reversible != nil {
		o.Reversible = *pol.Reversible
	}
	if pol.StashMinBytes != nil {
		o.StashMinBytes = *pol.StashMinBytes
	}

	// Explicit local env overrides.
	if _, set := os.LookupEnv(envNoDedup); set {
		o.Dedup = !envTrue(envNoDedup)
	}
	if _, set := os.LookupEnv(envReversible); set {
		o.Reversible = stash.Enabled()
	}
	if _, set := os.LookupEnv(envStashMin); set {
		o.StashMinBytes = stash.MinSize()
	}
	return o
}

func envTrue(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
