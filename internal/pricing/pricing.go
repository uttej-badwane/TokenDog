// Package pricing maps model identifiers to per-million-token rates across
// providers (Anthropic, OpenAI, Bedrock, Gemini). Modeled after
// ccusage/LiteLLM's pricing layer. The data is embedded — no network fetch at
// runtime — but structured so a future `td pricing refresh` could overwrite it
// from LiteLLM's public JSON.
//
// Sources of truth:
//   - https://docs.anthropic.com/en/docs/about-claude/pricing
//   - https://openai.com/api/pricing/
//   - https://aws.amazon.com/bedrock/pricing/
//   - https://github.com/BerriAI/litellm/blob/main/model_prices_and_context_window.json
//
// Last verified: 2026-06-30. Update DataVersion when prices change so callers
// can detect stale embedded data.
package pricing

import (
	"sort"
	"strings"
	"time"
)

// DataVersion ticks on each pricing update. Useful for cache invalidation
// and for `td gain` to footnote when its pricing data was last touched.
const DataVersion = "2026-06-30"

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
	// Opus 4.6/4.7/4.8 all price at $5/$25 standard. The >200K "1M-context"
	// tier is 2x input / 1.5x output, matching the Sonnet 4.6 convention below.
	"claude-opus-4-8": {
		Model:          "claude-opus-4-8",
		InputPerM:      5.0,
		OutputPerM:     25.0,
		Input1MPerM:    10.0,
		Output1MPerM:   37.50,
		CacheReadPerM:  0.50,
		CacheWritePerM: 6.25,
	},
	"claude-opus-4-7": {
		Model:          "claude-opus-4-7",
		InputPerM:      5.0,
		OutputPerM:     25.0,
		Input1MPerM:    10.0,
		Output1MPerM:   37.50,
		CacheReadPerM:  0.50,
		CacheWritePerM: 6.25,
	},
	"claude-opus-4-6": {
		Model:          "claude-opus-4-6",
		InputPerM:      5.0,
		OutputPerM:     25.0,
		Input1MPerM:    10.0,
		Output1MPerM:   37.50,
		CacheReadPerM:  0.50,
		CacheWritePerM: 6.25,
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

	// --- OpenAI ---
	"gpt-4o": {
		Model: "gpt-4o", InputPerM: 2.50, OutputPerM: 10.0, CacheReadPerM: 1.25,
	},
	"gpt-4o-mini": {
		Model: "gpt-4o-mini", InputPerM: 0.15, OutputPerM: 0.60, CacheReadPerM: 0.075,
	},
	"gpt-4.1": {
		Model: "gpt-4.1", InputPerM: 2.0, OutputPerM: 8.0, CacheReadPerM: 0.50,
	},
	"gpt-4.1-mini": {
		Model: "gpt-4.1-mini", InputPerM: 0.40, OutputPerM: 1.60, CacheReadPerM: 0.10,
	},
	"o3": {
		Model: "o3", InputPerM: 2.0, OutputPerM: 8.0, CacheReadPerM: 0.50,
	},
	"o4-mini": {
		Model: "o4-mini", InputPerM: 1.10, OutputPerM: 4.40, CacheReadPerM: 0.275,
	},

	// --- Amazon Bedrock (Claude-hosted + Amazon Nova). Keys are TokenDog
	// canonical ids; the Bedrock Converse model id resolves to these via
	// ProviderDefault until per-request model extraction lands. ---
	"bedrock-claude-sonnet": {
		Model: "bedrock-claude-sonnet", InputPerM: 3.0, OutputPerM: 15.0, CacheReadPerM: 0.30,
	},
	"bedrock-claude-haiku": {
		Model: "bedrock-claude-haiku", InputPerM: 0.80, OutputPerM: 4.0, CacheReadPerM: 0.08,
	},
	"amazon-nova-pro": {
		Model: "amazon-nova-pro", InputPerM: 0.80, OutputPerM: 3.20, CacheReadPerM: 0.20,
	},
	"amazon-nova-lite": {
		Model: "amazon-nova-lite", InputPerM: 0.06, OutputPerM: 0.24, CacheReadPerM: 0.015,
	},

	// --- Google Gemini (pricing-ready; no adapter yet) ---
	"gemini-2.0-flash": {
		Model: "gemini-2.0-flash", InputPerM: 0.10, OutputPerM: 0.40, CacheReadPerM: 0.025,
	},
	"gemini-1.5-pro": {
		Model: "gemini-1.5-pro", InputPerM: 1.25, OutputPerM: 5.0, CacheReadPerM: 0.3125,
	},
}

// datedRate is a rate snapshot tagged with the date it took effect. Models
// accumulate these only when a published price changes; see priceHistory.
type datedRate struct {
	From time.Time
	Rate Rate
}

// priceHistory holds superseded rate snapshots per canonical model id, oldest
// first. It is empty until a provider changes a price: at that point the prior
// rate is recorded here and currentFrom[model] is set to the changeover date,
// so spend in an earlier period stays priced at the rate that was in effect
// then instead of being silently re-priced at the current rate.
var priceHistory = map[string][]datedRate{}

// currentFrom records when each model's CURRENT rate (the entry in `rates`)
// took effect. A model absent from this map has only ever had one rate, which
// therefore applies to every timestamp.
var currentFrom = map[string]time.Time{}

// providerDefaults maps a provider id (as core.Provider.Name reports) to the
// model TokenDog prices its records at when the exact model isn't captured —
// the dominant agentic model for each provider. An empty/anthropic provider
// keeps the conservative Anthropic default.
var providerDefaults = map[string]string{
	"openai":  "gpt-4o",
	"bedrock": "bedrock-claude-sonnet",
	"gemini":  "gemini-2.0-flash",
}

// ProviderDefault returns the canonical model id used to price a record from
// the given provider when its exact model is unknown.
func ProviderDefault(provider string) string {
	if m, ok := providerDefaults[provider]; ok {
		return m
	}
	return DefaultModel
}

// LookupFor resolves a rate for a record's (provider, model). When the model
// is known it wins; otherwise the provider's default model rate is used (and
// ok=false flags the result as imputed).
func LookupFor(provider, model string) (Rate, bool) {
	if model != "" {
		if r, ok := Lookup(model); ok {
			return r, true
		}
	}
	r, _ := Lookup(ProviderDefault(provider))
	return r, false
}

// DefaultModel is the fallback when an analytics record has no model tag —
// either because it pre-dates per-model attribution or because the
// transcript wasn't reachable. Opus is the dominant agentic model, so its
// rate is the conservative choice for an unknown Anthropic record.
const DefaultModel = "claude-opus-4-8"

// resolveKey maps a model id to a canonical key in the rate table. Matches by
// exact id first, then by LONGEST prefix (so versioned ids like
// "claude-opus-4-7-20260418" or "gpt-4o-2024-08-06" resolve, and a
// "gpt-4o-mini-…" id doesn't collide with "gpt-4o"). Returns DefaultModel +
// false on miss so callers can mark the result as imputed.
func resolveKey(model string) (string, bool) {
	if _, ok := rates[model]; ok {
		return model, true
	}
	clean := strings.TrimPrefix(model, "anthropic/")
	prefixes := make([]string, 0, len(rates))
	for p := range rates {
		prefixes = append(prefixes, p)
	}
	sort.Slice(prefixes, func(i, j int) bool { return len(prefixes[i]) > len(prefixes[j]) })
	for _, prefix := range prefixes {
		if strings.HasPrefix(clean, prefix) {
			return prefix, true
		}
	}
	return DefaultModel, false
}

// Lookup returns the current Rate for a model id — equivalent to LookupAt at
// the present time. Returns the DefaultModel rate + ok=false on miss so callers
// can mark the result as imputed in user-facing output.
func Lookup(model string) (Rate, bool) {
	key, ok := resolveKey(model)
	return rates[key], ok
}

// LookupAt returns the Rate that applied to a model at time t. For a timestamp
// before the current rate took effect it consults priceHistory, so spend in a
// past period is priced at the rate in force then rather than at today's rate.
// A zero t means "now" and yields the current rate. ok mirrors Lookup
// (false ⇒ the id was unknown and DefaultModel was substituted).
func LookupAt(model string, t time.Time) (Rate, bool) {
	key, ok := resolveKey(model)
	return resolveDated(rates[key], currentFrom[key], priceHistory[key], t), ok
}

// resolveDated picks the rate effective at t from a model's current rate, the
// date that rate took effect (zero ⇒ it has always applied), and its ordered
// history (oldest first). Pure, so it can be tested without the package tables.
func resolveDated(cur Rate, curFrom time.Time, hist []datedRate, t time.Time) Rate {
	if t.IsZero() || curFrom.IsZero() || !t.Before(curFrom) {
		return cur
	}
	chosen := cur // fall back to the current rate if t predates all history
	for _, dr := range hist {
		if !dr.From.After(t) {
			chosen = dr.Rate // last match ≤ t wins, since hist is oldest-first
		}
	}
	return chosen
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
