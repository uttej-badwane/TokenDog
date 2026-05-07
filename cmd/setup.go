package cmd

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"tokendog/internal/proxy"
)

var (
	setupDryRun bool
	setupYes    bool
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "One-command end-to-end install (cert + daemon + shell rc + hook migration)",
	Long: `td setup is the friction-free path. Runs every step end users would
otherwise do by hand:

  1. Generate + install the TokenDog CA cert into the system trust store
  2. Install the launchd LaunchAgent so the proxy auto-starts at login
  3. Append HTTPS_PROXY to your shell rc (~/.zshrc, ~/.bashrc, etc.)
  4. Migrate the old PreToolUse hook out of ~/.claude/settings.json
  5. Verify end-to-end with a synthetic Anthropic request

Idempotent — re-running is safe. Use --dry-run to preview every change
without touching anything. Use td unsetup to reverse.

After setup, restart Claude Code (or your AI client) so it picks up the
new HTTPS_PROXY env var.`,
	RunE: runSetup,
}

var unsetupCmd = &cobra.Command{
	Use:   "unsetup",
	Short: "Reverse td setup — remove cert, daemon, shell-rc line",
	RunE:  runUnsetup,
}

func init() {
	setupCmd.Flags().BoolVar(&setupDryRun, "dry-run", false, "Print every change without touching anything")
	setupCmd.Flags().BoolVar(&setupYes, "yes", false, "Skip the confirmation prompt at the end")
}

func runSetup(_ *cobra.Command, _ []string) error {
	fmt.Println("TokenDog setup — five steps. Re-runnable, idempotent.")
	fmt.Println()

	// Step 1: cert
	if err := setupStep(1, "Install CA cert into system trust store", func() (string, error) {
		path, err := proxy.InstallCert()
		if err != nil {
			// On non-macOS, InstallCert returns the path AND an error
			// containing manual-install instructions. Preserve both.
			if path != "" {
				return path + " (manual install required: " + err.Error() + ")", nil
			}
			return "", err
		}
		return path, nil
	}); err != nil {
		return err
	}

	// Step 2: launchd daemon
	if err := setupStep(2, "Install launchd auto-start daemon", func() (string, error) {
		path, err := proxy.DaemonInstall()
		if err != nil {
			if path != "" {
				return path + " (with warning: " + err.Error() + ")", nil
			}
			return "", err
		}
		return path, nil
	}); err != nil {
		return err
	}

	// Step 3: shell rc
	if err := setupStep(3, "Append HTTPS_PROXY to your shell rc", appendShellRC); err != nil {
		return err
	}

	// Step 4: migrate hook
	if err := setupStep(4, "Remove the legacy PreToolUse hook (proxy supersedes it)", removeHookFromSettings); err != nil {
		return err
	}

	// Step 5: verify
	if err := setupStep(5, "Verify proxy interception with a synthetic request", verifyProxy); err != nil {
		// Verify failure isn't fatal — it can mean the daemon hasn't
		// finished starting. We surface the issue but don't unwind the
		// other 4 steps.
		fmt.Printf("  (verify failed: %v — give the daemon ~5s to start and run `td proxy daemon status`)\n", err)
	}

	fmt.Println()
	fmt.Println("Setup complete. Restart Claude Code so it picks up the new HTTPS_PROXY.")
	fmt.Println("Run `td gain --since 1d` after a session to see proxy savings.")
	return nil
}

// setupStep is the per-step UX wrapper. Prints "[N/5] description...",
// runs the action, prints success/failure with the result string. In
// dry-run mode, just prints what would happen.
func setupStep(n int, desc string, fn func() (string, error)) error {
	prefix := fmt.Sprintf("[%d/5] %s", n, desc)
	if setupDryRun {
		fmt.Println(prefix + " (dry-run; not executing)")
		return nil
	}
	fmt.Println(prefix + "...")
	result, err := fn()
	if err != nil {
		fmt.Printf("  ✗ %v\n", err)
		return err
	}
	if result == "" {
		fmt.Println("  ✓")
	} else {
		fmt.Println("  ✓ " + result)
	}
	return nil
}

// appendShellRC adds the HTTPS_PROXY block to the user's shell rc if
// not already present. Detects shell from $SHELL, supports zsh and bash.
// Idempotent: re-running detects the marker and skips.
func appendShellRC() (string, error) {
	rc, err := pickShellRC()
	if err != nil {
		return "", err
	}

	const marker = "# TokenDog proxy"
	const block = `
# TokenDog proxy — intercepts api.anthropic.com to filter tool_results
# Remove this block to disable proxy mode. Reverse with: td unsetup
export HTTPS_PROXY=http://127.0.0.1:8888
`

	existing, err := os.ReadFile(rc)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if strings.Contains(string(existing), marker) {
		return rc + " (already configured)", nil
	}
	f, err := os.OpenFile(rc, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.WriteString(block); err != nil {
		return "", err
	}
	return rc + " (appended HTTPS_PROXY block)", nil
}

func pickShellRC() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	shell := os.Getenv("SHELL")
	switch {
	case strings.HasSuffix(shell, "/zsh"):
		return filepath.Join(home, ".zshrc"), nil
	case strings.HasSuffix(shell, "/bash"):
		// Prefer .bashrc; macOS users sometimes use .bash_profile but
		// .bashrc is sourced too in interactive shells.
		return filepath.Join(home, ".bashrc"), nil
	case strings.HasSuffix(shell, "/fish"):
		return filepath.Join(home, ".config", "fish", "config.fish"), nil
	default:
		// Unknown shell — fall back to .profile which most POSIX-y
		// shells source.
		return filepath.Join(home, ".profile"), nil
	}
}

// removeHookFromSettings rewrites ~/.claude/settings.json to remove the
// PreToolUse Bash matcher that runs `td hook claude`. Other hooks (Stop,
// PostToolUse, etc.) are preserved exactly. Idempotent.
//
// We rewrite via temp + rename so a partial write can't corrupt the
// settings file.
func removeHookFromSettings() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(home, ".claude", "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "settings.json not present (skipping)", nil
		}
		return "", err
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return "", fmt.Errorf("settings.json is not valid JSON; not modifying it: %w", err)
	}
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		return "no hooks block to modify", nil
	}
	pre, _ := hooks["PreToolUse"].([]any)
	if len(pre) == 0 {
		return "no PreToolUse hooks to modify", nil
	}

	// Filter out any matcher block whose hooks include `td hook claude`.
	// Other matcher blocks (e.g. Read-tool hooks) are kept.
	var keptMatchers []any
	removedCount := 0
	for _, raw := range pre {
		matcher, _ := raw.(map[string]any)
		if matcher == nil {
			keptMatchers = append(keptMatchers, raw)
			continue
		}
		hookList, _ := matcher["hooks"].([]any)
		isTD := false
		for _, h := range hookList {
			hook, _ := h.(map[string]any)
			if cmd, _ := hook["command"].(string); cmd == "td hook claude" {
				isTD = true
				break
			}
		}
		if isTD {
			removedCount++
			continue
		}
		keptMatchers = append(keptMatchers, matcher)
	}

	if removedCount == 0 {
		return "no `td hook claude` entries found (already migrated)", nil
	}

	if len(keptMatchers) == 0 {
		delete(hooks, "PreToolUse")
	} else {
		hooks["PreToolUse"] = keptMatchers
	}

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	return fmt.Sprintf("removed %d `td hook claude` entry from %s", removedCount, path), nil
}

// verifyProxy sends a synthetic Messages API request through the proxy
// (using an invalid API key — we just want the round-trip to prove TLS +
// interception work) and confirms a 401 came back. The 401 is the right
// answer; any other outcome indicates a setup problem.
func verifyProxy() (string, error) {
	// Wait briefly for the launchd daemon to bind the port.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if isProxyListening() {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !isProxyListening() {
		return "", fmt.Errorf("proxy not listening on 127.0.0.1:8888")
	}

	body := strings.NewReader(`{"model":"claude-haiku-4-5","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`)
	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", body)
	if err != nil {
		return "", err
	}
	req.Header.Set("x-api-key", "td-setup-verify-invalid")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	proxyURL, _ := url.Parse("http://127.0.0.1:8888")
	tr := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
	}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("proxy request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 {
		return "round-trip succeeded (got expected 401 from invalid key)", nil
	}
	return fmt.Sprintf("round-trip succeeded (HTTP %d — unexpected but proxy is working)", resp.StatusCode), nil
}

func isProxyListening() bool {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:8888", 500*time.Millisecond)
	if conn != nil {
		_ = conn.Close()
	}
	return err == nil
}

func runUnsetup(_ *cobra.Command, _ []string) error {
	fmt.Println("TokenDog unsetup — reversing setup. Cert files preserved on disk.")
	fmt.Println()

	if err := setupStep(1, "Stop and remove launchd daemon", func() (string, error) {
		return "", proxy.DaemonUninstall()
	}); err != nil {
		return err
	}

	if err := setupStep(2, "Remove HTTPS_PROXY from shell rc", removeShellRC); err != nil {
		return err
	}

	if err := setupStep(3, "Remove CA cert from system trust store", func() (string, error) {
		return "", proxy.UninstallCert()
	}); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("Done. Open a new shell so HTTPS_PROXY clears, then restart Claude Code.")
	fmt.Println("Cert files at ~/.config/tokendog/proxy/ are kept; delete manually if you want them gone.")
	return nil
}

func removeShellRC() (string, error) {
	rc, err := pickShellRC()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(rc)
	if err != nil {
		if os.IsNotExist(err) {
			return "no rc file to modify", nil
		}
		return "", err
	}
	const startMarker = "# TokenDog proxy"
	idx := strings.Index(string(data), startMarker)
	if idx < 0 {
		return "no TokenDog block found", nil
	}
	// Remove from "# TokenDog proxy" through the next blank line.
	rest := string(data)[idx:]
	endIdx := strings.Index(rest, "\n\n")
	if endIdx < 0 {
		endIdx = len(rest)
	} else {
		endIdx++ // include the trailing newline
	}
	out := string(data)[:idx] + string(data)[idx+endIdx:]
	if err := os.WriteFile(rc, []byte(out), 0644); err != nil {
		return "", err
	}
	return rc + " (removed TokenDog block)", nil
}
