// Package pricing maps Anthropic model identifiers to per-million-token
// rates. Modeled after ccusage/LiteLLM's pricing layer but scoped to the
// models TokenDog actually targets (Claude). The data is embedded — no
// network fetch at runtime — but structured so a future `td pricing
// refresh` could overwrite it from LiteLLM's public JSON.
//
// Sources of truth:
//   - https://docs.anthropic.com/en/docs/about-claude/pricing
//   - https://github.com/BerriAI/litellm/blob/main/model_prices_and_context_window.json
//
// Last verified: 2026-05. Update DataVersion when prices change so callers
// can detect stale embedded data.
package pricing

import "strings"

// DataVersion ticks on each pricing update. Useful for cache invalidation
// and for `td gain` to footnote when its pricing data was last touched.
const DataVersion = "2026-05-06"

// Rate holds per-million-token costs for a single model. Output rates are
// not used by td gain (tool-result data flows in as input on subsequent
// turns) but kept here for completeness and future use.
type Rate struct {
	Model          string  // canonical model id, e.g. "claude-opus-4-7"
	InputPerM      float64 // $/M for cache_miss input tokens (standard tier)
	OutputPerM     float64 // $/M for output tokens
	Input1MPerM    float64 // $/M for input above 200K context (1M-context tier); 0 if N/A
	Output1MPerM   float64 // $/M for output above 200K context
	CacheReadPerM  float64 // $/M for cache_read_input_tokens (typically 10% of input)
	CacheWritePerM float64 // $/M for cache_creation_input_tokens (typically 125% of input)
}

// rates holds the embedded pricing snapshot. Keys are canonical model ids
// matching what Anthropic's `message.model` field reports in transcripts.
var rates = map[string]Rate{
	"claude-opus-4-7": {
		Model:          "claude-opus-4-7",
		InputPerM:      15.0,
		OutputPerM:     75.0,
		Input1MPerM:    30.0,
		Output1MPerM:   150.0,
		CacheReadPerM:  1.50,
		CacheWritePerM: 18.75,
	},
	"claude-opus-4-6": {
		Model:          "claude-opus-4-6",
		InputPerM:      15.0,
		OutputPerM:     75.0,
		CacheReadPerM:  1.50,
		CacheWritePerM: 18.75,
	},
	"claude-sonnet-4-6": {
		Model:          "claude-sonnet-4-6",
		InputPerM:      3.0,
		OutputPerM:     15.0,
		Input1MPerM:    6.0,
		Output1MPerM:   22.50,
		CacheReadPerM:  0.30,
		CacheWritePerM: 3.75,
	},
	"claude-sonnet-4-5": {
		Model:          "claude-sonnet-4-5",
		InputPerM:      3.0,
		OutputPerM:     15.0,
		CacheReadPerM:  0.30,
		CacheWritePerM: 3.75,
	},
	"claude-haiku-4-5": {
		Model:          "claude-haiku-4-5",
		InputPerM:      0.80,
		OutputPerM:     4.0,
		CacheReadPerM:  0.08,
		CacheWritePerM: 1.0,
	},
	"claude-3-5-sonnet": {
		Model:          "claude-3-5-sonnet",
		InputPerM:      3.0,
		OutputPerM:     15.0,
		CacheReadPerM:  0.30,
		CacheWritePerM: 3.75,
	},
	"claude-3-opus": {
		Model:          "claude-3-opus",
		InputPerM:      15.0,
		OutputPerM:     75.0,
		CacheReadPerM:  1.50,
		CacheWritePerM: 18.75,
	},
	"claude-3-haiku": {
		Model:          "claude-3-haiku",
		InputPerM:      0.25,
		OutputPerM:     1.25,
		CacheReadPerM:  0.03,
		CacheWritePerM: 0.30,
	},
}

// DefaultModel is the fallback when an analytics record has no model tag —
// either because it pre-dates per-model attribution or because the
// transcript wasn't reachable. Opus 4.7 errs on the side of overstating
// cost (vs. understating with Haiku), making "saved cost" estimates
// conservative-low rather than speculatively high.
const DefaultModel = "claude-opus-4-7"

// Lookup returns the Rate for a model id. Matches by exact id first, then
// falls back to a prefix match (so versioned ids like "claude-opus-4-7-20260418"
// still resolve). Returns the DefaultModel rate + ok=false on miss so
// callers can mark the result as imputed in user-facing output.
func Lookup(model string) (Rate, bool) {
	if r, ok := rates[model]; ok {
		return r, true
	}
	// Prefix match for version-suffixed ids like "claude-opus-4-7-20260418"
	// or "anthropic/claude-haiku-4-5".
	clean := strings.TrimPrefix(model, "anthropic/")
	for prefix, r := range rates {
		if strings.HasPrefix(clean, prefix) {
			return r, true
		}
	}
	return rates[DefaultModel], false
}

// Models returns the canonical list of known model ids. Stable order so
// renderers can iterate predictably.
func Models() []string {
	out := make([]string, 0, len(rates))
	for k := range rates {
		out = append(out, k)
	}
	// Sort with most-expensive first so per-model tables read top-down by impact.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && rates[out[j]].InputPerM > rates[out[j-1]].InputPerM; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// USDForInput returns the cost in USD for `tokens` input tokens at this
// model's standard-tier rate. For sessions known to use the 1M context
// premium, callers should use USDForInput1M instead — we don't auto-detect
// since that requires per-turn context-size data.
func (r Rate) USDForInput(tokens int) float64 {
	return float64(tokens) / 1_000_000 * r.InputPerM
}

// USDForInput1M is the premium-tier variant for >200K context input. Falls
// back to standard rate when the model has no separate 1M-tier price.
func (r Rate) USDForInput1M(tokens int) float64 {
	rate := r.Input1MPerM
	if rate == 0 {
		rate = r.InputPerM
	}
	return float64(tokens) / 1_000_000 * rate
}
