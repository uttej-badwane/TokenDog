package proxy

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"

	"github.com/elazarl/goproxy"
)

// DefaultListenAddr is where the proxy binds. Configurable via TD_PROXY_ADDR.
const DefaultListenAddr = "127.0.0.1:8888"

// AnthropicHostSuffix matches the host we MITM. We intercept the API
// endpoint specifically — any other HTTPS traffic the client sends
// flows through transparently (the proxy doesn't terminate TLS unless
// the host matches).
const AnthropicHostSuffix = "api.anthropic.com"

// ListenAddr resolves the listen address from env or default.
func ListenAddr() string {
	if v := os.Getenv("TD_PROXY_ADDR"); v != "" {
		return v
	}
	return DefaultListenAddr
}

// RequestHandler is the swap-in point for filtering. The default
// implementation in server.go is passthrough (logs but doesn't modify).
// Phase 3 swaps in filtering logic.
type RequestHandler func(req *http.Request, body []byte) (modifiedBody []byte, err error)

// PassthroughHandler logs the request shape but doesn't touch the body.
// Used by phase-2 commits to prove the plumbing.
func PassthroughHandler(req *http.Request, body []byte) ([]byte, error) {
	fmt.Fprintf(os.Stderr, "[td proxy] %s %s (%d bytes)\n", req.Method, req.URL.Path, len(body))
	return body, nil
}

// Server holds the running proxy state. Created via NewServer; started
// via ListenAndServe.
type Server struct {
	addr    string
	proxy   *goproxy.ProxyHttpServer
	handler RequestHandler
}

// NewServer wires up goproxy with our CA cert + the request handler.
// Caller can swap RequestHandler before ListenAndServe — useful for
// tests and for the phase-3 filtering layer.
func NewServer(handler RequestHandler) (*Server, error) {
	exists, err := CAExists()
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("CA cert not found — run `td proxy install-cert` first")
	}
	certPath, _ := CACertPath()
	keyPath, _ := CAKeyPath()
	caCert, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("read CA cert: %w", err)
	}
	caKey, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read CA key: %w", err)
	}
	caCertX509, err := tls.X509KeyPair(caCert, caKey)
	if err != nil {
		return nil, fmt.Errorf("parse CA: %w", err)
	}
	goproxy.GoproxyCa = caCertX509
	// Reset the per-host TLS config so MITM uses our new CA. goproxy
	// computes leaf certs on demand using GoproxyCa as the signing key.
	goproxy.OkConnect = &goproxy.ConnectAction{Action: goproxy.ConnectAccept, TLSConfig: goproxy.TLSConfigFromCA(&caCertX509)}
	goproxy.MitmConnect = &goproxy.ConnectAction{Action: goproxy.ConnectMitm, TLSConfig: goproxy.TLSConfigFromCA(&caCertX509)}
	goproxy.HTTPMitmConnect = &goproxy.ConnectAction{Action: goproxy.ConnectHTTPMitm, TLSConfig: goproxy.TLSConfigFromCA(&caCertX509)}
	goproxy.RejectConnect = &goproxy.ConnectAction{Action: goproxy.ConnectReject, TLSConfig: goproxy.TLSConfigFromCA(&caCertX509)}

	gp := goproxy.NewProxyHttpServer()
	gp.Verbose = false

	// MITM only Anthropic — every other host's CONNECT is tunneled
	// through transparently. This minimizes our trust footprint.
	hostRe := regexp.MustCompile(`^api\.anthropic\.com(:\d+)?$`)
	gp.OnRequest(goproxy.ReqHostMatches(hostRe)).
		HandleConnect(goproxy.AlwaysMitm)

	// Per-request handler for the intercepted (decrypted) traffic.
	gp.OnRequest(goproxy.UrlHasPrefix("/v1/messages")).DoFunc(
		func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
			body, err := readAllBody(req)
			if err != nil {
				ctx.Logf("read body: %v", err)
				return req, nil
			}
			modified, err := handler(req, body)
			if err != nil {
				ctx.Logf("handler error: %v — forwarding original body", err)
				modified = body
			}
			req = replaceBody(req, modified)
			return req, nil
		},
	)

	return &Server{
		addr:    ListenAddr(),
		proxy:   gp,
		handler: handler,
	}, nil
}

// ListenAndServe blocks running the proxy. Errors only if listen fails.
// Caller is responsible for graceful shutdown (e.g. via signal handler).
func (s *Server) ListenAndServe() error {
	fmt.Fprintf(os.Stderr, "TokenDog proxy listening on %s\n", s.addr)
	fmt.Fprintf(os.Stderr, "  HTTPS_PROXY=http://%s claude  # to route a single invocation\n", s.addr)
	fmt.Fprintf(os.Stderr, "  Set HTTPS_PROXY in your shell rc to make it permanent.\n")
	return http.ListenAndServe(s.addr, s.proxy)
}

// Helpers — body read/replace dance because http.Request.Body is io.Reader.
// Kept short; full-body buffering is fine here because Anthropic Messages
// requests are bounded by their context-window size.

// readAllBody slurps the entire request body. Bounded by maxBody to
// defend against pathological input. Uses io.ReadAll under a LimitReader
// rather than rolling our own loop so we get io.EOF semantics for free —
// rolling our own previously returned a non-io.EOF error which broke
// downstream HTTP transport (it treats non-io.EOF as "connection bad,
// retry" and the retry hit phantom failures).
func readAllBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	defer r.Body.Close()
	const maxBody = 32 << 20
	return io.ReadAll(io.LimitReader(r.Body, maxBody))
}

// replaceBody substitutes the request body. Uses io.NopCloser around a
// bytes.Reader — both are stdlib, both correctly return io.EOF.
func replaceBody(r *http.Request, body []byte) *http.Request {
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	r.Header.Set("Content-Length", fmt.Sprintf("%d", len(body)))
	return r
}
