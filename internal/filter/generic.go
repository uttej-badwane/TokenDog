package filter

import (
	"encoding/json"
	"strings"
)

// Generic is the content-type fallback. The per-tool filters in this package
// are keyed by binary name, so output from any command TokenDog doesn't
// recognize passes through untouched. Generic closes that gap by sniffing the
// *shape* of the output rather than the command that produced it, catching
// the long tail of unhandled binaries.
//
// It returns (compacted, true) only when it both recognized the shape and
// produced strictly fewer bytes; otherwise (raw, false). Like every filter it
// is lossless — it restructures, never drops.
//
// Today it handles one shape: a single JSON value (object or array), which it
// re-marshals without indentation. That's the most common machine-readable
// output a no-filter command emits — REST responses via curl/httpie, config
// dumps, `--output json` from CLIs we don't have a dedicated filter for.
// Other shapes can be added here as separate sniffers.
func Generic(raw string) (string, bool) {
	if out, ok := genericJSON(raw); ok {
		return out, true
	}
	return raw, false
}

// genericJSON compacts raw when it is exactly one JSON value. The "exactly
// one" check (via Decode + More) is what keeps it safe: partial output, log
// lines that merely start with `{`, or JSON streams with trailing noise all
// fail to parse cleanly and pass through unchanged.
func genericJSON(raw string) (string, bool) {
	t := strings.TrimSpace(raw)
	if t == "" {
		return raw, false
	}
	// Only attempt structural JSON — objects and arrays. Bare scalars
	// ("true", numbers, quoted strings) aren't worth the round-trip and are
	// rarely a whole tool output.
	if t[0] != '{' && t[0] != '[' {
		return raw, false
	}
	dec := json.NewDecoder(strings.NewReader(t))
	var v any
	if err := dec.Decode(&v); err != nil {
		return raw, false
	}
	// Reject anything with a second value or trailing non-whitespace tokens —
	// we only compact a clean, single document.
	if dec.More() {
		return raw, false
	}
	out, err := json.Marshal(v)
	if err != nil {
		return raw, false
	}
	if len(out) >= len(raw) {
		// Already compact (or smaller pretty-print edge cases) — no win.
		return raw, false
	}
	return string(out), true
}
