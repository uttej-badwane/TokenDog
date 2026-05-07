package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"tokendog/internal/proxy"
)

var proxyCmd = &cobra.Command{
	Use:   "proxy",
	Short: "Run TokenDog as an HTTPS proxy (alternative to PreToolUse hook)",
	Long: `The proxy mode is the architectural alternative to the hook approach.
Instead of wrapping every Bash command with ` + "`td <tool>`" + ` (visible
in shell history and Claude transcripts), the proxy intercepts the HTTPS
requests Claude Code (or any AI client) sends to api.anthropic.com,
filters tool_result content blocks in the payload, and forwards the
modified request.

Pros over hook mode:
  - Invisible to your shell, your transcript, and your AI workflow
  - Filters everything the model sees: Bash, file reads, MCP responses,
    web fetches — not just Bash output (~30-50% of bill vs ~5-8%)

Cons:
  - Requires installing a self-signed CA cert in your system trust store
  - The proxy daemon must be running for AI traffic to be intercepted
  - Adds a few ms of latency per API call

Setup (one-time):
  td proxy install-cert       # generates + installs a local CA cert
  td proxy start              # runs the proxy daemon

  # Add to your shell rc (~/.zshrc):
  export HTTPS_PROXY=http://localhost:8888

  # Restart Claude Code. It will route through the proxy.

Then remove the PreToolUse hook from ~/.claude/settings.json — proxy
mode replaces it.`,
}

var proxyInstallCertCmd = &cobra.Command{
	Use:   "install-cert",
	Short: "Generate and install TokenDog's local CA cert into the system trust store",
	RunE:  runProxyInstallCert,
}

var proxyUninstallCertCmd = &cobra.Command{
	Use:   "uninstall-cert",
	Short: "Remove TokenDog's CA cert from the system trust store",
	RunE:  runProxyUninstallCert,
}

var proxyStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Run the proxy in the foreground (Ctrl-C to stop)",
	Long: `Starts the HTTPS proxy bound to localhost:8888 (override with
TD_PROXY_ADDR=host:port). The proxy MITMs traffic to api.anthropic.com
ONLY — every other host is tunneled through unchanged, minimizing the
trust footprint.

For background daemon mode see ` + "`td proxy daemon`" + ` (planned, not yet
shipped). For now: ` + "`nohup td proxy start &`" + ` works fine.`,
	RunE: runProxyStart,
}

func init() {
	proxyCmd.AddCommand(proxyInstallCertCmd)
	proxyCmd.AddCommand(proxyUninstallCertCmd)
	proxyCmd.AddCommand(proxyStartCmd)
}

func runProxyInstallCert(_ *cobra.Command, _ []string) error {
	path, err := proxy.InstallCert()
	if err != nil {
		// On non-macOS this is expected — InstallCert returns the cert
		// path AND an error with manual-install instructions. Print them.
		if path != "" {
			fmt.Println(err.Error())
			return nil
		}
		return err
	}
	fmt.Printf("✓ CA cert generated and installed: %s\n", path)
	fmt.Println("  Next: run `td proxy start` and set HTTPS_PROXY=http://localhost:8888")
	return nil
}

func runProxyUninstallCert(_ *cobra.Command, _ []string) error {
	if err := proxy.UninstallCert(); err != nil {
		return err
	}
	fmt.Println("✓ CA cert removed from system trust store")
	fmt.Println("  Cert files at ~/.config/tokendog/proxy/ are kept; delete manually if you want.")
	return nil
}

func runProxyStart(_ *cobra.Command, _ []string) error {
	srv, err := proxy.NewServer(proxy.FilterHandler)
	if err != nil {
		return err
	}
	return srv.ListenAndServe()
}
