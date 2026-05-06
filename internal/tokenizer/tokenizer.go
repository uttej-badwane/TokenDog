// Package tokenizer counts tokens for cost analytics. Anthropic does not
// publish a Go tokenizer, so we use OpenAI's cl100k_base via tiktoken-go as
// a proxy — empirically within ~10% of Anthropic's tokenization for English
// + code, which is sufficient for cost estimation in `td gain`.
//
// On first use, tiktoken-go downloads the cl100k vocab (~1.5MB) and caches
// it under the user's tiktoken cache dir. If the download fails (offline,
// firewall) we fall back to a bytes/4 estimate so analytics never blocks.
package tokenizer

import (
	"sync"

	"github.com/pkoukk/tiktoken-go"
)

const encoding = "cl100k_base"

var (
	once   sync.Once
	enc    *tiktoken.Tiktoken
	encErr error
	loadOK bool
)

// load lazily initializes the tokenizer the first time Count is called. We
// don't fail loudly on init errors — analytics is best-effort.
func load() {
	once.Do(func() {
		enc, encErr = tiktoken.GetEncoding(encoding)
		loadOK = encErr == nil && enc != nil
	})
}

// Count returns the token count for s. Uses cl100k_base as an Anthropic
// proxy. On tokenizer-init failure, falls back to a (bytes+3)/4 estimate.
func Count(s string) int {
	load()
	if !loadOK {
		return estimateFromBytes(len(s))
	}
	return len(enc.Encode(s, nil, nil))
}

// CountBytes is the cheap fallback used by callers that don't want the
// tiktoken init cost (e.g. tight loops). For long-form analytics, prefer
// Count.
func CountBytes(n int) int { return estimateFromBytes(n) }

// Available reports whether the real tokenizer loaded. `td gain` uses this
// to footnote estimates as "approximate" when the fallback is in play.
func Available() bool {
	load()
	return loadOK
}

func estimateFromBytes(n int) int {
	if n <= 0 {
		return 0
	}
	return (n + 3) / 4
}
