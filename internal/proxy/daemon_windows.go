//go:build windows

package proxy

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const TaskName = "TokenDogProxy"

// DaemonInstall registers a Windows Scheduled Task that runs `td proxy
// start` at user logon. Idempotent: the /F flag overwrites an existing
// task with the same name.
//
// The task runs at HIGHEST privilege level so it can bind the proxy port
// without UAC prompts on subsequent logins. The first `schtasks /Create`
// call does require an elevated prompt if UAC is strict; in that case
// the user will see a consent dialog.
func DaemonInstall() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return "", err
	}

	// Create (or overwrite) the ONLOGON task.
	createCmd := exec.Command("schtasks",
		"/Create",
		"/TN", TaskName,
		"/TR", fmt.Sprintf(`"%s" proxy start`, exe),
		"/SC", "ONLOGON",
		"/RL", "HIGHEST",
		"/F",
	)
	if out, err := createCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("schtasks /Create: %w\n%s", err, out)
	}

	// Start immediately so the user doesn't need to log out and back in.
	if out, err := exec.Command("schtasks", "/Run", "/TN", TaskName).CombinedOutput(); err != nil {
		// Non-fatal — the task is registered and will run at next logon.
		return TaskName, fmt.Errorf("task registered but failed to start immediately: %w\n%s", err, out)
	}
	return TaskName, nil
}

// DaemonUninstall ends any running instance and deletes the scheduled task.
func DaemonUninstall() error {
	// /End — stop if running (best-effort; ignore errors).
	_ = exec.Command("schtasks", "/End", "/TN", TaskName).Run()

	out, err := exec.Command("schtasks", "/Delete", "/TN", TaskName, "/F").CombinedOutput()
	if err != nil {
		// Treat "The system cannot find the file" (task not registered)
		// as success — the user's goal is achieved either way.
		if strings.Contains(string(out), "cannot find") || strings.Contains(string(out), "does not exist") {
			return nil
		}
		return fmt.Errorf("schtasks /Delete: %w\n%s", err, out)
	}
	return nil
}

// DaemonStatus queries the task's current status via schtasks CSV output.
func DaemonStatus() (string, error) {
	out, err := exec.Command("schtasks",
		"/Query", "/TN", TaskName,
		"/FO", "CSV", "/NH",
	).Output()
	if err != nil {
		return "not installed (task not found) — run: td proxy daemon install", nil
	}

	// CSV line: "TaskName","Next Run Time","Status"
	// Strip quotes and split.
	line := strings.TrimSpace(string(out))
	line = strings.ReplaceAll(line, `"`, "")
	fields := strings.Split(line, ",")
	if len(fields) >= 3 {
		status := strings.TrimSpace(fields[2])
		return fmt.Sprintf("task status: %s", status), nil
	}
	return fmt.Sprintf("task found: %s", line), nil
}
