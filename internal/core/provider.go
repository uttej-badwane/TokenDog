package core

// Provider adapts one wire protocol (Anthropic Messages, OpenAI Chat
// Completions, …) to the engine. An adapter parses a request body into a
// Conversation, runs Compress, writes any replacements back into the original
// payload, and returns the rewritten body plus the savings to record.
//
// Adapters register themselves via Register in their init(); frontends (the
// MITM proxy, the explicit-base_url gateway, an SDK middleware) call Dispatch
// and never need to know which provider handled the request.
type Provider interface {
	// Name identifies the provider (for logging / analytics labels).
	Name() string
	// Match reports whether this provider handles a request at the given URL
	// path (e.g. "/v1/messages" for Anthropic).
	Match(path string) bool
	// Compress rewrites body, returning the new body and the savings to
	// record. When nothing applied — or the body isn't a compressible
	// request for this provider — it returns the original body and nil
	// savings. It must never return a malformed body; on any internal error
	// it returns the original unchanged.
	Compress(body []byte, opts Options) (out []byte, savings []Saving, err error)
}

var providers []Provider

// Register adds a provider to the dispatch set. Called from adapter init().
func Register(p Provider) { providers = append(providers, p) }

// Providers returns the registered providers (for diagnostics/tests).
func Providers() []Provider { return providers }

// Dispatch routes a request body to the first provider whose Match accepts
// the path. It returns the rewritten body, the savings, the matched provider's
// name (so the frontend can tokenize/price per provider), and any error. When
// no provider matches, the body is returned unchanged with an empty provider —
// the safe default for paths TokenDog doesn't compress.
func Dispatch(path string, body []byte, opts Options) (out []byte, savings []Saving, provider string, err error) {
	for _, p := range providers {
		if p.Match(path) {
			out, savings, err = p.Compress(body, opts)
			return out, savings, p.Name(), err
		}
	}
	return body, nil, "", nil
}
