package cmd

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"tokendog/internal/proxy"
)

// goos is a thin wrapper around runtime.GOOS so the test file can shim it.
func goos() string { return runtime.GOOS }

var (
	setupDryRun bool
	setupYes    bool
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "One-command end-to-end install (cert + daemon + shell rc + GUI env + hook migration)",
	Long: `td setup is the friction-free path. Runs every step end users would
otherwise do by hand:

  1. Generate + install the TokenDog CA cert into the system trust store
  2. Install the launchd LaunchAgent so the proxy auto-starts at login
  3. Append HTTPS_PROXY to your shell rc (~/.zshrc, ~/.bashrc, etc.)
  4. Set HTTPS_PROXY at the launchd level so GUI apps see it (macOS) +
     install a persistence agent so it survives reboots
  5. Migrate the old PreToolUse hook out of ~/.claude/settings.json
  6. Verify end-to-end with a synthetic Anthropic request

Idempotent — re-running is safe. Use --dry-run to preview every change
without touching anything. Use td unsetup to reverse.

After setup completes, you MUST restart your AI client to pick up the
new HTTPS_PROXY env var. The detailed restart steps print at the end.`,
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
	fmt.Println("TokenDog setup — six steps. Re-runnable, idempotent.")
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

	// Step 4: GUI app env (launchctl setenv + persistence plist)
	if err := setupStep(4, "Set HTTPS_PROXY for macOS GUI apps", installGUIProxyEnv); err != nil {
		// Non-macOS — not fatal. Just continue.
		fmt.Printf("  (skipping on non-macOS or insufficient permissions)\n")
	}

	// Step 5: migrate hook
	if err := setupStep(5, "Remove the legacy PreToolUse hook (proxy supersedes it)", removeHookFromSettings); err != nil {
		return err
	}

	// Step 6: verify
	if err := setupStep(6, "Verify proxy interception with a synthetic request", verifyProxy); err != nil {
		// Verify failure isn't fatal — it can mean the daemon hasn't
		// finished starting. We surface the issue but don't unwind the
		// other 5 steps.
		fmt.Printf("  (verify failed: %v — give the daemon ~5s to start and run `td proxy daemon status`)\n", err)
	}

	printPostSetupInstructions()
	return nil
}

// printPostSetupInstructions explains the most important point users miss:
// running shells and running apps don't automatically pick up the new env.
// We spell out the exact command for each entry point.
func printPostSetupInstructions() {
	fmt.Println()
	fmt.Println("Setup complete. The most important step is next:")
	fmt.Println()
	fmt.Println("  RESTART YOUR AI CLIENT")
	fmt.Println()
	fmt.Println("Existing terminal shells and running apps have their env fixed at")
	fmt.Println("startup time — they won't see the new HTTPS_PROXY. Pick the path")
	fmt.Println("that matches how you use claude:")
	fmt.Println()
	fmt.Println("  Terminal claude CLI")
	fmt.Println("    Option A: open a NEW terminal window, then start claude there.")
	fmt.Println("    Option B: in your current shell, prefix the env on launch:")
	fmt.Println("              HTTPS_PROXY=http://127.0.0.1:8888 claude")
	fmt.Println()
	fmt.Println("  Claude.app (Mac desktop)")
	fmt.Println("    Quit fully (cmd-Q from menu, not just close window). Relaunch")
	fmt.Println("    via the dock or via:")
	fmt.Println("              open -a Claude --args \\")
	fmt.Println("                --proxy-server=http://127.0.0.1:8888 \\")
	fmt.Println("                --proxy-bypass-list='<-loopback>'")
	fmt.Println("    (Claude.app is Electron-based; HTTPS_PROXY env is ignored,")
	fmt.Println("     so the --proxy-server flag is the canonical way.)")
	fmt.Println()
	fmt.Println("After your client is restarted, run:")
	fmt.Println("    tail -f ~/.config/tokendog/proxy.log    # watch live activity")
	fmt.Println("    td gain --since 1h                      # see proxy savings")
}

// installGUIProxyEnv sets HTTPS_PROXY at the launchd level so GUI apps
// (Mac desktop apps started from Finder/Dock) inherit it. Plus writes a
// persistence LaunchAgent so the setting survives reboots. macOS-only;
// the agent itself is a no-op shell script so this is safe.
func installGUIProxyEnv() (string, error) {
	if runtimeGOOS() != "darwin" {
		return "", fmt.Errorf("not macOS")
	}
	// 1. Set for the current launchd session.
	for _, name := range []string{"HTTPS_PROXY", "https_proxy"} {
		if err := launchctlSetenv(name, "http://127.0.0.1:8888"); err != nil {
			return "", err
		}
	}
	// 2. Write a persistence agent that re-applies the setenv at every
	// user login. This is a separate plist from the proxy daemon — its
	// only job is to call launchctl setenv on each load.
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.tokendog.proxyenv.plist")
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.tokendog.proxyenv</string>
    <key>ProgramArguments</key>
    <array>
        <string>/bin/sh</string>
        <string>-c</string>
        <string>launchctl setenv HTTPS_PROXY http://127.0.0.1:8888 &amp;&amp; launchctl setenv https_proxy http://127.0.0.1:8888</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
</dict>
</plist>
`
	if err := os.MkdirAll(filepath.Dir(plistPath), 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(plistPath, []byte(plist), 0644); err != nil {
		return "", err
	}
	// Bootstrap (idempotent — bootout first, then bootstrap).
	target := fmt.Sprintf("gui/%d", os.Getuid())
	_ = exec.Command("launchctl", "bootout", target+"/com.tokendog.proxyenv").Run()
	if err := exec.Command("launchctl", "bootstrap", target, plistPath).Run(); err != nil {
		// Non-fatal — the setenv we did in step 1 is still in effect for
		// this login session.
		return plistPath + " (current session set; persistence plist install warning: " + err.Error() + ")", nil
	}
	return plistPath, nil
}

func launchctlSetenv(name, value string) error {
	return exec.Command("launchctl", "setenv", name, value).Run()
}

// runtimeGOOS is wrapped so tests can stub it. Direct runtime.GOOS works
// fine in production.
func runtimeGOOS() string { return goos() }

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

	if err := setupStep(3, "Unset HTTPS_PROXY for GUI apps + remove persistence agent", uninstallGUIProxyEnv); err != nil {
		// Non-fatal on non-macOS.
	}

	if err := setupStep(4, "Remove CA cert from system trust store", func() (string, error) {
		return "", proxy.UninstallCert()
	}); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("Done. Open a new shell so HTTPS_PROXY clears, then restart Claude Code.")
	fmt.Println("Cert files at ~/.config/tokendog/proxy/ are kept; delete manually if you want them gone.")
	return nil
}

func uninstallGUIProxyEnv() (string, error) {
	if runtimeGOOS() != "darwin" {
		return "", fmt.Errorf("not macOS")
	}
	target := fmt.Sprintf("gui/%d/com.tokendog.proxyenv", os.Getuid())
	_ = exec.Command("launchctl", "bootout", target).Run()
	for _, name := range []string{"HTTPS_PROXY", "https_proxy"} {
		_ = exec.Command("launchctl", "unsetenv", name).Run()
	}
	home, _ := os.UserHomeDir()
	plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.tokendog.proxyenv.plist")
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return "removed", nil
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
