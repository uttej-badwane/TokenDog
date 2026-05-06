package pricing

import "testing"

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
