//go:build linux

package proxy

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const ServiceName = "tokendog-proxy"

// DaemonInstall writes a systemd user unit and enables it so the proxy
// starts automatically when the user logs in. Idempotent: re-running
// refreshes the unit file and restarts the service.
//
// Uses systemd --user (no root required). The unit file lands at
// ~/.config/systemd/user/tokendog-proxy.service.
//
// Prerequisites: systemd must be running as a user manager (most modern
// Linux desktops; check with `systemctl --user status`). Headless servers
// without a user session need `loginctl enable-linger $USER` once.
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
	unitDir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(unitDir, 0755); err != nil {
		return "", fmt.Errorf("creating systemd unit dir: %w", err)
	}
	logDir := filepath.Join(home, ".config", "tokendog")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return "", err
	}

	unitPath := filepath.Join(unitDir, ServiceName+".service")
	unit := buildSystemdUnit(exe)
	if err := os.WriteFile(unitPath, []byte(unit), 0644); err != nil {
		return "", fmt.Errorf("writing unit file: %w", err)
	}

	// daemon-reload is required after writing/updating a unit file.
	if out, err := exec.Command("systemctl", "--user", "daemon-reload").CombinedOutput(); err != nil {
		return unitPath, fmt.Errorf("systemctl --user daemon-reload: %w\n%s", err, out)
	}

	// enable + start (idempotent: --now restarts if already running).
	if out, err := exec.Command("systemctl", "--user", "enable", "--now", ServiceName).CombinedOutput(); err != nil {
		return unitPath, fmt.Errorf("systemctl --user enable --now: %w\n%s\n"+
			"Hint: if running on a headless server, run `loginctl enable-linger $USER` first.", err, out)
	}

	return unitPath, nil
}

// DaemonUninstall stops, disables, and removes the systemd user unit.
func DaemonUninstall() error {
	// Stop + disable (best-effort; ignore "not loaded" errors).
	_ = exec.Command("systemctl", "--user", "disable", "--now", ServiceName).Run()

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	unitPath := filepath.Join(home, ".config", "systemd", "user", ServiceName+".service")
	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing unit file: %w", err)
	}
	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	return nil
}

// DaemonStatus returns a human-readable line about the service state.
func DaemonStatus() (string, error) {
	// `systemctl --user is-active` exits 0 when active, non-zero otherwise.
	isActive := exec.Command("systemctl", "--user", "is-active", ServiceName)
	activeOut, _ := isActive.Output()
	state := strings.TrimSpace(string(activeOut))

	switch state {
	case "active":
		// Fetch PID from the MainPID property.
		propOut, _ := exec.Command("systemctl", "--user", "show",
			ServiceName, "--property=MainPID", "--value").Output()
		pid := strings.TrimSpace(string(propOut))
		return fmt.Sprintf("running (PID %s, unit %s.service)", pid, ServiceName), nil
	case "inactive":
		return fmt.Sprintf("installed but not running — start with: systemctl --user start %s", ServiceName), nil
	case "failed":
		return fmt.Sprintf("failed — check logs: journalctl --user -u %s -n 50", ServiceName), nil
	case "activating":
		return "starting up...", nil
	default:
		// Check whether the unit file exists at all.
		home, _ := os.UserHomeDir()
		unitPath := filepath.Join(home, ".config", "systemd", "user", ServiceName+".service")
		if _, err := os.Stat(unitPath); os.IsNotExist(err) {
			return "not installed — run: td proxy daemon install", nil
		}
		return fmt.Sprintf("state: %s (unit file present but not loaded)", state), nil
	}
}

// buildSystemdUnit generates the [Unit]/[Service]/[Install] stanzas for a
// user-session proxy daemon. Restarts automatically after failures with a
// 5-second backoff to prevent thrashing on config errors.
func buildSystemdUnit(binPath string) string {
	return fmt.Sprintf(`[Unit]
Description=TokenDog HTTPS proxy — filters tool output to save AI tokens
After=network.target

[Service]
ExecStart=%s proxy start
Restart=on-failure
RestartSec=5
StandardOutput=append:%%h/.config/tokendog/proxy.log
StandardError=append:%%h/.config/tokendog/proxy.log

[Install]
WantedBy=default.target
`, binPath)
}
