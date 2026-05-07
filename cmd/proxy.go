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

var proxyDaemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Manage the proxy as an auto-starting background service",
	Long: `Subcommands install/uninstall/status the proxy as a launchd LaunchAgent
on macOS. The agent starts at login and respawns if the proxy crashes.
Linux and Windows print platform-specific manual setup instructions.

  td proxy daemon install     # write plist, load via launchctl
  td proxy daemon status      # is it running? what's the PID?
  td proxy daemon uninstall   # bootout + remove plist`,
}

var proxyDaemonInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the proxy as a launchd LaunchAgent (macOS)",
	RunE:  runProxyDaemonInstall,
}

var proxyDaemonUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Stop the LaunchAgent and remove the plist",
	RunE:  runProxyDaemonUninstall,
}

var proxyDaemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Report whether the LaunchAgent is loaded + the proxy's PID",
	RunE:  runProxyDaemonStatus,
}

func init() {
	proxyCmd.AddCommand(proxyInstallCertCmd)
	proxyCmd.AddCommand(proxyUninstallCertCmd)
	proxyCmd.AddCommand(proxyStartCmd)
	proxyCmd.AddCommand(proxyDaemonCmd)
	proxyDaemonCmd.AddCommand(proxyDaemonInstallCmd)
	proxyDaemonCmd.AddCommand(proxyDaemonUninstallCmd)
	proxyDaemonCmd.AddCommand(proxyDaemonStatusCmd)
}

func runProxyDaemonInstall(_ *cobra.Command, _ []string) error {
	plistPath, err := proxy.DaemonInstall()
	if err != nil {
		return err
	}
	fmt.Printf("✓ LaunchAgent installed: %s\n", plistPath)
	fmt.Println("  Auto-starts at login. Respawns if the proxy crashes (10s throttle).")
	fmt.Println("  Logs: ~/.config/tokendog/proxy.log")
	fmt.Println("  Status: td proxy daemon status")
	return nil
}

func runProxyDaemonUninstall(_ *cobra.Command, _ []string) error {
	if err := proxy.DaemonUninstall(); err != nil {
		return err
	}
	fmt.Println("✓ LaunchAgent removed. The proxy will not auto-start at next login.")
	return nil
}

func runProxyDaemonStatus(_ *cobra.Command, _ []string) error {
	status, err := proxy.DaemonStatus()
	if err != nil {
		return err
	}
	fmt.Println(status)
	return nil
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
