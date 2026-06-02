// Package tokenizer counts tokens for cost analytics across providers.
//
// Different providers tokenize differently, so a single encoder under-counts
// for some. We map a provider to its closest available tiktoken encoding:
//
//   - cl100k_base — Anthropic (no official Go tokenizer; ~10% proxy), older
//     OpenAI (gpt-4 / 3.5), and the default for anything without a better
//     match (Bedrock-hosted Claude, Gemini).
//   - o200k_base  — modern OpenAI (gpt-4o family), whose vocab differs enough
//     from cl100k that using cl100k would mis-report token savings.
//
// Encoders are loaded lazily and cached per encoding. tiktoken-go downloads
// each vocab (~1.5-2MB) on first use and caches it under the user's tiktoken
// cache dir. On any load failure (offline, firewall) we fall back to a
// bytes/4 estimate so analytics never blocks.
package tokenizer

import (
	"sync"

	"github.com/pkoukk/tiktoken-go"
)

const defaultEncoding = "cl100k_base"

type encoder struct {
	enc *tiktoken.Tiktoken
	ok  bool
}

var (
	mu    sync.Mutex
	cache = map[string]*encoder{}
)

// get returns the cached encoder for an encoding name, loading it once.
func get(name string) *encoder {
	if name == "" {
		name = defaultEncoding
	}
	mu.Lock()
	defer mu.Unlock()
	if e, ok := cache[name]; ok {
		return e
	}
	enc, err := tiktoken.GetEncoding(name)
	e := &encoder{enc: enc, ok: err == nil && enc != nil}
	cache[name] = e
	return e
}

// Count returns the token count for s using the default (cl100k) encoding —
// the Anthropic proxy. Preserved for callers that don't carry a provider.
func Count(s string) int { return CountWith(defaultEncoding, s) }

// CountWith returns the token count for s under a specific encoding. An empty
// encoding means the default. Falls back to a (bytes+3)/4 estimate if the
// encoder couldn't load.
func CountWith(encoding, s string) int {
	e := get(encoding)
	if !e.ok {
		return estimateFromBytes(len(s))
	}
	return len(e.enc.Encode(s, nil, nil))
}

// EncodingFor maps a provider id (as reported by a core.Provider) to its
// tiktoken encoding. Best-effort: providers without an official Go tokenizer
// use the closest available vocab.
func EncodingFor(provider string) string {
	switch provider {
	case "openai":
		return "o200k_base"
	default:
		// anthropic, bedrock (predominantly Claude), gemini, and "" all use
		// cl100k as the closest available proxy.
		return defaultEncoding
	}
}

// CountBytes is the cheap fallback used by callers that don't want the
// tiktoken init cost (e.g. tight loops). For long-form analytics, prefer
// Count / CountWith.
func CountBytes(n int) int { return estimateFromBytes(n) }

// Available reports whether the default real tokenizer loaded. `td gain` uses
// this to footnote estimates as "approximate" when the fallback is in play.
func Available() bool { return get(defaultEncoding).ok }

func estimateFromBytes(n int) int {
	if n <= 0 {
		return 0
	}
	return (n + 3) / 4
}
