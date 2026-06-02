package core

import (
	"testing"

	"tokendog/internal/policy"
)

func TestOptionsDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // no policy file
	t.Setenv("TD_NO_DEDUP", "")
	t.Setenv("TD_REVERSIBLE", "")
	t.Setenv("TD_STASH_MIN", "")
	// LookupEnv treats "" as set, so unset them fully by relying on a clean
	// temp HOME and explicit-empty meaning "set to empty" — emulate true
	// default by NOT setting overrides below. Here we just assert baseline.
	o := OptionsFromEnv()
	if !o.Dedup {
		t.Error("dedup should default on")
	}
}

func TestOptionsPolicyBaseline(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := policy.Save(policy.Policy{
		Dedup:         policy.Bool(false),
		Reversible:    policy.Bool(true),
		StashMinBytes: policy.Int(512),
	}); err != nil {
		t.Fatal(err)
	}
	o := OptionsFromEnv()
	if o.Dedup {
		t.Error("policy dedup=false should disable dedup")
	}
	if !o.Reversible {
		t.Error("policy reversible=true should enable reversible")
	}
	if o.StashMinBytes != 512 {
		t.Errorf("policy stash_min should apply, got %d", o.StashMinBytes)
	}
}

func TestOptionsEnvOverridesPolicy(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := policy.Save(policy.Policy{
		Dedup:      policy.Bool(false), // policy says off…
		Reversible: policy.Bool(true),  // policy says on…
	}); err != nil {
		t.Fatal(err)
	}
	// …but the developer's explicit env vars win locally.
	t.Setenv("TD_NO_DEDUP", "0")   // re-enable dedup
	t.Setenv("TD_REVERSIBLE", "0") // disable reversible

	o := OptionsFromEnv()
	if !o.Dedup {
		t.Error("explicit TD_NO_DEDUP=0 should override policy and enable dedup")
	}
	if o.Reversible {
		t.Error("explicit TD_REVERSIBLE=0 should override policy and disable reversible")
	}
}
