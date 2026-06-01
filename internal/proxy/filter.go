package proxy

import (
	"fmt"
	"net/http"
	"os"

	_ "tokendog/internal/adapter/anthropic" // registers the Anthropic provider
	"tokendog/internal/analytics"
	"tokendog/internal/core"
)

// FilterHandler is the proxy's RequestHandler. It is now a thin frontend over
// the provider-neutral engine: it hands the request body to core.Dispatch
// (which routes by URL path to the matching provider adapter), records any
// savings, and returns the rewritten body. All the compression intelligence
// lives in internal/core and internal/adapter — the proxy just supplies the
// transport and the analytics sink.
//
// Cache safety and "last message only" semantics are enforced inside the
// adapters/engine, not here.
func FilterHandler(req *http.Request, body []byte) ([]byte, error) {
	out, savings, err := core.Dispatch(req.URL.Path, body, core.OptionsFromEnv())
	if err != nil {
		// Engine trouble — never send a payload we can't trust. Fall back to
		// the original bytes.
		return body, nil
	}
	for _, s := range savings {
		recordProxySaving(s.Label, s.Original, s.Result)
	}
	return out, nil
}

// recordProxySaving writes a proxy-mode analytics record. Same Record schema
// used by hook mode so `td gain` aggregates both transparently.
func recordProxySaving(command, raw, filtered string) {
	rec := analytics.NewRecord("proxy: "+command, raw, filtered, 0)
	if err := analytics.Save(rec); err != nil {
		fmt.Fprintf(os.Stderr, "[td proxy] analytics save failed: %v\n", err)
	}
}
