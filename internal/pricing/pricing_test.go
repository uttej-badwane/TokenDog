package pricing

import (
	"testing"
	"time"
)

func TestLookupExact(t *testing.T) {
	r, ok := Lookup("claude-opus-4-7")
	if !ok {
		t.Fatal("expected ok=true for known model")
	}
	if r.InputPerM != 15.0 {
		t.Errorf("InputPerM = %v, want 15.0", r.InputPerM)
	}
}

func TestLookupVersionedSuffix(t *testing.T) {
	// Anthropic ships dated model ids; prefix match must catch them.
	r, ok := Lookup("claude-opus-4-7-20260418")
	if !ok {
		t.Fatal("expected ok=true for version-suffixed model")
	}
	if r.Model != "claude-opus-4-7" {
		t.Errorf("Model = %q, want canonical claude-opus-4-7", r.Model)
	}
}

func TestLookupAnthropicPrefix(t *testing.T) {
	r, ok := Lookup("anthropic/claude-haiku-4-5")
	if !ok {
		t.Fatal("expected ok=true after stripping anthropic/ prefix")
	}
	if r.InputPerM != 0.80 {
		t.Errorf("InputPerM = %v, want 0.80", r.InputPerM)
	}
}

func TestLookupUnknownFallsBackWithFalse(t *testing.T) {
	r, ok := Lookup("gpt-4-turbo")
	if ok {
		t.Error("expected ok=false for non-Claude model")
	}
	// Returns DefaultModel rate so callers don't have to handle nil.
	if r.Model != DefaultModel {
		t.Errorf("expected fallback to %q, got %q", DefaultModel, r.Model)
	}
}

func TestUSDForInput(t *testing.T) {
	r, _ := Lookup("claude-opus-4-7")
	got := r.USDForInput(1_000_000)
	if got != 15.0 {
		t.Errorf("USDForInput(1M) = %v, want 15.0", got)
	}
}

func TestUSDForInput1MFallback(t *testing.T) {
	// claude-opus-4-6 has no separate 1M-tier price; should fall back to standard.
	r, _ := Lookup("claude-opus-4-6")
	if r.Input1MPerM != 0 {
		t.Skip("claude-opus-4-6 now has 1M tier; rewrite this test")
	}
	got := r.USDForInput1M(1_000_000)
	if got != r.InputPerM {
		t.Errorf("USDForInput1M fell back to %v, want standard %v", got, r.InputPerM)
	}
}

func TestModelsSortedByCost(t *testing.T) {
	models := Models()
	if len(models) < 2 {
		t.Fatal("expected at least 2 models")
	}
	prev, _ := Lookup(models[0])
	for _, m := range models[1:] {
		curr, _ := Lookup(m)
		if curr.InputPerM > prev.InputPerM {
			t.Errorf("Models() order broken: %s ($%v) before %s ($%v)",
				models[0], prev.InputPerM, m, curr.InputPerM)
		}
		prev = curr
	}
}

func TestOpenAIAndBedrockRatesPresent(t *testing.T) {
	for _, m := range []string{"gpt-4o", "gpt-4o-mini", "bedrock-claude-sonnet", "amazon-nova-lite", "gemini-2.0-flash"} {
		if r, ok := Lookup(m); !ok || r.InputPerM <= 0 {
			t.Errorf("expected a rate for %q, got %+v ok=%v", m, r, ok)
		}
	}
}

func TestLookupLongestPrefixWins(t *testing.T) {
	// "gpt-4o-mini-2024-…" must resolve to gpt-4o-mini, NOT gpt-4o.
	r, ok := Lookup("gpt-4o-mini-2024-07-18")
	if !ok || r.Model != "gpt-4o-mini" {
		t.Errorf("versioned gpt-4o-mini resolved to %q (ok=%v), want gpt-4o-mini", r.Model, ok)
	}
	r, ok = Lookup("gpt-4o-2024-08-06")
	if !ok || r.Model != "gpt-4o" {
		t.Errorf("versioned gpt-4o resolved to %q, want gpt-4o", r.Model)
	}
}

func TestResolveDated(t *testing.T) {
	cur := Rate{Model: "m", InputPerM: 20}
	old := Rate{Model: "m", InputPerM: 10}
	older := Rate{Model: "m", InputPerM: 5}

	mar := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	jan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	hist := []datedRate{{From: time.Time{}, Rate: older}, {From: jan, Rate: old}}

	cases := []struct {
		name    string
		curFrom time.Time
		hist    []datedRate
		at      time.Time
		want    float64
	}{
		{"zero time yields current", mar, hist, time.Time{}, 20},
		{"no curFrom always current", time.Time{}, hist, jan, 20},
		{"on/after curFrom is current", mar, hist, mar, 20},
		{"after curFrom is current", mar, hist, mar.AddDate(0, 1, 0), 20},
		{"between history entries", mar, hist, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), 10},
		{"before jan uses the epoch (oldest) rate", mar, hist, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), 5},
		{"no history falls back to current", mar, nil, jan, 20},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := resolveDated(cur, c.curFrom, c.hist, c.at).InputPerM
			if got != c.want {
				t.Errorf("resolveDated = %v, want %v", got, c.want)
			}
		})
	}
}

func TestLookupAtMatchesLookupWhenNoHistory(t *testing.T) {
	// With the shipped (single-snapshot) tables, LookupAt at any time must match
	// Lookup — the dated path is inert until a price change is recorded.
	for _, m := range []string{"claude-opus-4-7", "gpt-4o", "unknown-model"} {
		now, _ := Lookup(m)
		at, _ := LookupAt(m, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
		if now != at {
			t.Errorf("LookupAt(%q) = %+v, want %+v (no history yet)", m, at, now)
		}
	}
}

func TestProviderDefaultAndLookupFor(t *testing.T) {
	if ProviderDefault("openai") != "gpt-4o" {
		t.Error("openai default should be gpt-4o")
	}
	if ProviderDefault("bedrock") != "bedrock-claude-sonnet" {
		t.Error("bedrock default should be bedrock-claude-sonnet")
	}
	if ProviderDefault("") != DefaultModel {
		t.Error("empty provider should keep the Anthropic default")
	}
	// Unknown model on a known provider → provider default, imputed.
	r, ok := LookupFor("openai", "")
	if ok || r.Model != "gpt-4o" {
		t.Errorf("LookupFor(openai, '') = %q ok=%v, want gpt-4o imputed", r.Model, ok)
	}
	// Known model wins regardless of provider.
	r, ok = LookupFor("openai", "gpt-4o-mini")
	if !ok || r.Model != "gpt-4o-mini" {
		t.Errorf("LookupFor with explicit model should resolve exactly, got %q", r.Model)
	}
}
