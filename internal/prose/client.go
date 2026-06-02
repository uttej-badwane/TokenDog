// Package prose provides the frontend-side client for an optional prose
// compression sidecar (an extractive ONNX prose model behind a small HTTP
// service — see experiments/prose-sidecar). It returns a core.ProseFunc so the
// engine can stay free of HTTP/network — the proxy and gateway inject one of
// these into core.Options.
//
// Wiring is opt-in via env: set TD_PROSE_ENDPOINT to the sidecar URL. Without
// it, FromEnv returns nil and the prose route is simply off (the reversible
// pass falls back to its head/tail preview). The compressor is lossy, so the
// engine only ever calls it inside the reversible pass, where the original is
// stashed and recoverable.
package prose

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"time"

	"tokendog/internal/core"
)

const (
	envEndpoint  = "TD_PROSE_ENDPOINT"  // e.g. http://127.0.0.1:8071/compress
	envThreshold = "TD_PROSE_THRESHOLD" // keep-prob cutoff; default in the safe band
	envTimeoutMS = "TD_PROSE_TIMEOUT_MS"

	defaultThreshold = 0.6  // ≤0.7 kept 100% of facts in the eval; conservative
	defaultTimeoutMS = 2000 // prose runs on the proxy path — bound it hard
)

// FromEnv returns a core.ProseFunc backed by the sidecar at TD_PROSE_ENDPOINT,
// or nil when that env var is unset (prose route disabled). The returned func
// is best-effort: any error, timeout, or non-200 yields (.,false) so the
// engine cleanly falls back to its head/tail preview.
func FromEnv() core.ProseFunc {
	endpoint := os.Getenv(envEndpoint)
	if endpoint == "" {
		return nil
	}
	threshold := defaultThreshold
	if v, err := strconv.ParseFloat(os.Getenv(envThreshold), 64); err == nil && v > 0 {
		threshold = v
	}
	timeout := time.Duration(defaultTimeoutMS) * time.Millisecond
	if v, err := strconv.Atoi(os.Getenv(envTimeoutMS)); err == nil && v > 0 {
		timeout = time.Duration(v) * time.Millisecond
	}
	client := &http.Client{Timeout: timeout}

	return func(text string) (string, bool) {
		reqBody, err := json.Marshal(map[string]any{"text": text, "threshold": threshold})
		if err != nil {
			return "", false
		}
		resp, err := client.Post(endpoint, "application/json", bytes.NewReader(reqBody))
		if err != nil {
			return "", false
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return "", false
		}
		var out struct {
			Compressed string `json:"compressed"`
		}
		if json.NewDecoder(resp.Body).Decode(&out) != nil || out.Compressed == "" {
			return "", false
		}
		return out.Compressed, true
	}
}
