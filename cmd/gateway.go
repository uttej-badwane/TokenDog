package cmd

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"

	// Register the provider adapters so Dispatch can route by path.
	_ "tokendog/internal/adapter/anthropic"
	_ "tokendog/internal/adapter/bedrock"
	_ "tokendog/internal/adapter/openai"
	"tokendog/internal/analytics"
	"tokendog/internal/core"
)

var (
	gatewayPort     int
	gatewayUpstream string
)

var gatewayCmd = &cobra.Command{
	Use:   "gateway",
	Short: "Run a compression reverse-proxy your SDK opts into via base_url (no CA cert)",
	Long: `An explicit-opt-in alternative to the MITM proxy. Instead of installing a
root CA and intercepting TLS, you point your LLM SDK's base URL at this
gateway over plain localhost HTTP; it compresses tool output and forwards to
the real provider over normal TLS.

  td gateway --port 8099 --upstream https://api.anthropic.com

Then:

  ANTHROPIC_BASE_URL=http://127.0.0.1:8099 claude
  # or, in code:
  client = Anthropic(base_url="http://127.0.0.1:8099")
  client = OpenAI(base_url="http://127.0.0.1:8099/v1")

Why this exists: a CA-installing MITM is a non-starter for most security
teams. An explicit base_url is a deliberate, auditable opt-in — no
interception of traffic the user didn't redirect, no trust-store changes.
The same engine (internal/core) routes by request path: Anthropic
(/v1/messages), OpenAI (/v1/chat/completions), and Amazon Bedrock Converse
(/model/{id}/converse). Point --upstream at the matching provider:

  td gateway --upstream https://bedrock-runtime.us-east-1.amazonaws.com`,
	RunE: runGateway,
}

func init() {
	gatewayCmd.Flags().IntVar(&gatewayPort, "port", 8099, "Local port to listen on")
	gatewayCmd.Flags().StringVar(&gatewayUpstream, "upstream", "https://api.anthropic.com",
		"Upstream provider base URL to forward to")
}

func runGateway(_ *cobra.Command, _ []string) error {
	upstream, err := url.Parse(gatewayUpstream)
	if err != nil {
		return fmt.Errorf("invalid --upstream %q: %w", gatewayUpstream, err)
	}
	if upstream.Scheme == "" || upstream.Host == "" {
		return fmt.Errorf("--upstream must be an absolute URL like https://api.anthropic.com")
	}

	rp := httputil.NewSingleHostReverseProxy(upstream)
	origDirector := rp.Director
	rp.Director = func(r *http.Request) {
		origDirector(r)
		// Make the request look like it originated for the upstream host so
		// TLS SNI and Host header line up.
		r.Host = upstream.Host
	}

	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil && (r.Method == http.MethodPost || r.Method == http.MethodPut) {
			body, err := io.ReadAll(r.Body)
			r.Body.Close()
			if err == nil {
				out, savings, derr := core.Dispatch(r.URL.Path, body, core.OptionsFromEnv())
				if derr != nil {
					out = body // never forward something we couldn't trust
				}
				for _, s := range savings {
					recordGatewaySaving(s)
				}
				r.Body = io.NopCloser(bytes.NewReader(out))
				r.ContentLength = int64(len(out))
				r.Header.Set("Content-Length", strconv.Itoa(len(out)))
			}
		}
		rp.ServeHTTP(w, r)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", gatewayPort)
	fmt.Printf("td gateway listening on http://%s → %s\n", addr, upstream)
	fmt.Println("Point your SDK's base_url here (no CA cert needed). Ctrl-C to stop.")
	return http.ListenAndServe(addr, http.HandlerFunc(handler))
}

// recordGatewaySaving mirrors the proxy's analytics write, tagged "gateway:"
// so `td gain` can distinguish the deployment source.
func recordGatewaySaving(s core.Saving) {
	rec := analytics.NewRecord("gateway: "+s.Label, s.Original, s.Result, 0)
	_ = analytics.Save(rec)
}
