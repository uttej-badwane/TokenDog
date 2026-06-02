// Package policy holds a centrally-managed configuration a platform team can
// distribute to a fleet of developer machines (via MDM, dotfiles, or
// `td fleet pull <url>`). It lets an org govern the engine's behavior — which
// optional passes are on, what the reversible-stash threshold is — without
// every developer setting environment variables by hand.
//
// Precedence is: an explicitly-set environment variable (the user's local
// override) wins over policy, which wins over the built-in default. So policy
// sets the managed baseline but never traps a developer who needs to deviate.
//
// The struct uses pointers so "unset" is distinct from "false": a field left
// out of policy.json leaves that knob at its default rather than forcing it
// off.
package policy

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Policy is the on-disk managed configuration. Every field is optional.
type Policy struct {
	// Dedup governs cross-message dedup (default on).
	Dedup *bool `json:"dedup,omitempty"`
	// Reversible governs reversible stash+preview (default off).
	Reversible *bool `json:"reversible,omitempty"`
	// StashMinBytes is the minimum output size before reversible stashing
	// kicks in (default 2048).
	StashMinBytes *int `json:"stash_min_bytes,omitempty"`
}

// Empty reports whether the policy sets nothing.
func (p Policy) Empty() bool {
	return p.Dedup == nil && p.Reversible == nil && p.StashMinBytes == nil
}

// Path returns the policy file location.
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "tokendog", "policy.json"), nil
}

// Load reads the managed policy. Best-effort: a missing or malformed file
// yields an empty Policy (all defaults), never an error — a broken policy
// file must not break compression.
func Load() Policy {
	path, err := Path()
	if err != nil {
		return Policy{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Policy{}
	}
	var p Policy
	if err := json.Unmarshal(data, &p); err != nil {
		return Policy{}
	}
	return p
}

// Save writes the policy to disk (used by `td fleet pull`).
func Save(p Policy) error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Bool returns a pointer to b, for setting optional Policy fields.
func Bool(b bool) *bool { return &b }

// Int returns a pointer to i, for setting optional Policy fields.
func Int(i int) *int { return &i }
