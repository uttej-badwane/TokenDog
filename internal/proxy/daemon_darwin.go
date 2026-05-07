//go:build darwin

package proxy

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// LaunchAgentLabel is the canonical launchd job identifier. Stable across
// versions so install/uninstall/status all target the same record.
const LaunchAgentLabel = "com.tokendog.proxy"

// PlistPath returns the canonical location of the LaunchAgent plist
// under the user's home — never under /Library/LaunchDaemons (which
// would be system-wide and require sudo).
func PlistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", LaunchAgentLabel+".plist"), nil
}

// DaemonInstall writes the launchd plist and bootstraps it. Idempotent:
// re-running with an existing plist refreshes it (in case the binary
// path moved) and reloads the agent.
//
// We use launchctl bootstrap (modern macOS, 10.10+) rather than the
// deprecated launchctl load. Bootstrap targets the GUI domain for the
// current user — same scope as Apple's "login items" menu.
func DaemonInstall() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return "", err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	logPath := filepath.Join(home, ".config", "tokendog", "proxy.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		return "", err
	}

	plistDir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(plistDir, 0755); err != nil {
		return "", err
	}
	plistPath, err := PlistPath()
	if err != nil {
		return "", err
	}

	// If there's already a loaded job at this label, bootout first so
	// the bootstrap below picks up any path/log changes. Ignore errors —
	// "not loaded" is fine, anything else surfaces below when bootstrap
	// fails.
	_ = bootout()

	plist := buildPlist(exe, logPath)
	if err := os.WriteFile(plistPath, []byte(plist), 0644); err != nil {
		return "", fmt.Errorf("write plist: %w", err)
	}

	if err := bootstrap(plistPath); err != nil {
		return plistPath, err
	}
	return plistPath, nil
}

// DaemonUninstall stops the agent and removes the plist. Idempotent —
// "not loaded" / "no such file" both treated as success.
func DaemonUninstall() error {
	_ = bootout()
	plistPath, err := PlistPath()
	if err != nil {
		return err
	}
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// DaemonStatus returns a human-readable line about the agent's state.
// Three observable states:
//   - not installed (no plist, not loaded)
//   - installed but not running (plist exists, launchctl shows no PID)
//   - running (plist exists, PID is positive)
func DaemonStatus() (string, error) {
	plistPath, err := PlistPath()
	if err != nil {
		return "", err
	}
	plistExists := false
	if _, err := os.Stat(plistPath); err == nil {
		plistExists = true
	}
	pid, loaded := launchctlPID()
	switch {
	case !plistExists && !loaded:
		return "not installed", nil
	case plistExists && !loaded:
		return fmt.Sprintf("plist present at %s but agent not loaded — run `td proxy daemon install` to (re)load", plistPath), nil
	case loaded && pid <= 0:
		return "loaded but not running (likely crashed; check ~/.config/tokendog/proxy.log)", nil
	default:
		return fmt.Sprintf("running (PID %d, plist %s)", pid, plistPath), nil
	}
}

// buildPlist generates the LaunchAgent XML. Embedded as a Go raw string
// rather than text/template — the substitutions are simple and a
// template just adds a dep with no win.
//
// Key choices:
//   - RunAtLoad: true   — start when the user logs in
//   - KeepAlive:  true  — respawn if the proxy exits
//   - ThrottleInterval: 10  — minimum 10s between respawns to avoid
//     fork-bombing on a misconfigured port
//   - StandardOut/ErrorPath — single combined log under TD's config dir
func buildPlist(binPath, logPath string) string {
	const tmpl = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>proxy</string>
        <string>start</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>ThrottleInterval</key>
    <integer>10</integer>
    <key>StandardOutPath</key>
    <string>%s</string>
    <key>StandardErrorPath</key>
    <string>%s</string>
</dict>
</plist>
`
	return fmt.Sprintf(tmpl, LaunchAgentLabel, binPath, logPath, logPath)
}

// bootstrap loads the plist into the current GUI session. Equivalent to
// `launchctl bootstrap gui/$UID <plist>` — the modern replacement for
// the deprecated `launchctl load`.
func bootstrap(plistPath string) error {
	target := "gui/" + strconv.Itoa(os.Getuid())
	cmd := exec.Command("launchctl", "bootstrap", target, plistPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Some macOS versions (older 10.10/10.11) don't have bootstrap.
		// Fall back to load — same effect, deprecated since 10.11.
		cmd2 := exec.Command("launchctl", "load", "-w", plistPath)
		out2, err2 := cmd2.CombinedOutput()
		if err2 != nil {
			return fmt.Errorf("launchctl bootstrap: %w\n%s\nfallback launchctl load: %v\n%s",
				err, out, err2, out2)
		}
	}
	return nil
}

// bootout removes the agent from the current GUI session. Best-effort.
func bootout() error {
	target := "gui/" + strconv.Itoa(os.Getuid()) + "/" + LaunchAgentLabel
	cmd := exec.Command("launchctl", "bootout", target)
	if err := cmd.Run(); err == nil {
		return nil
	}
	// Fallback for older macOS.
	plistPath, perr := PlistPath()
	if perr != nil {
		return perr
	}
	return exec.Command("launchctl", "unload", plistPath).Run()
}

// launchctlPID returns the agent's PID if running, plus a "loaded" flag
// indicating whether the agent appears in launchctl's list at all.
// `launchctl list <label>` outputs a property-list-ish blob; grep is
// fine for our purpose.
func launchctlPID() (pid int, loaded bool) {
	cmd := exec.Command("launchctl", "list", LaunchAgentLabel)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, false
	}
	loaded = true
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		// Look for `"PID" = N;`. PID is missing/0 when not running.
		if strings.HasPrefix(line, `"PID" = `) {
			rest := strings.TrimSuffix(strings.TrimPrefix(line, `"PID" = `), ";")
			n, err := strconv.Atoi(strings.TrimSpace(rest))
			if err == nil {
				return n, true
			}
		}
	}
	return 0, true
}
