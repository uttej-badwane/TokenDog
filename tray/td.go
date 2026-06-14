package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// errTDNotFound is returned when no `td` binary can be located.
var errTDNotFound = errors.New("td not found")

// tdPath resolves the `td` CLI. A tray app inherits a minimal PATH (it's often
// started by the desktop session, not a login shell), so we probe the usual
// install locations explicitly after PATH. TOKENDOG_BIN overrides everything.
func tdPath() string {
	if o := os.Getenv("TOKENDOG_BIN"); o != "" && isExec(o) {
		return o
	}
	name := "td"
	if runtime.GOOS == "windows" {
		name = "td.exe"
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	for _, c := range fallbacks() {
		if isExec(c) {
			return c
		}
	}
	return ""
}

// fallbacks lists common install locations per OS, tried in order after PATH.
func fallbacks() []string {
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "windows" {
		return []string{
			filepath.Join(home, "go", "bin", "td.exe"),
			filepath.Join(home, "scoop", "shims", "td.exe"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "WinGet", "Links", "td.exe"),
		}
	}
	return []string{
		"/opt/homebrew/bin/td", // Apple Silicon Homebrew (dev on mac)
		"/usr/local/bin/td",    // Intel Homebrew / Linuxbrew / manual
		"/usr/bin/td",
		filepath.Join(home, "go", "bin", "td"),
		filepath.Join(home, ".local", "bin", "td"),
	}
}

func isExec(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// fetchReport runs `td spend --json` and decodes it.
func fetchReport() (*Report, error) {
	path := tdPath()
	if path == "" {
		return nil, errTDNotFound
	}
	out, err := exec.Command(path, "spend", "--json").Output()
	if err != nil {
		return nil, err
	}
	var r Report
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, fmt.Errorf("decoding td output: %w", err)
	}
	return &r, nil
}

// openReport launches the full breakdown (`td gain --by-model`) in a terminal.
// Best-effort and OS-specific; failures are swallowed.
func openReport() {
	td := tdPath()
	if td == "" {
		return
	}
	switch runtime.GOOS {
	case "windows":
		// `start "" cmd /k …` opens a new console that stays open.
		_ = exec.Command("cmd", "/c", "start", "", "cmd", "/k", td, "gain", "--by-model").Start()
	case "darwin":
		script := fmt.Sprintf(
			"tell application \"Terminal\" to do script \"%s gain --by-model\"\n"+
				"tell application \"Terminal\" to activate", td)
		_ = exec.Command("osascript", "-e", script).Start()
	default: // linux / *bsd
		cmdline := fmt.Sprintf("%s gain --by-model; echo; read -n1 -r -p 'Press any key to close…'", td)
		for _, term := range []string{"x-terminal-emulator", "gnome-terminal", "konsole", "xfce4-terminal", "xterm"} {
			if p, err := exec.LookPath(term); err == nil {
				_ = exec.Command(p, "-e", "sh", "-c", cmdline).Start()
				return
			}
		}
	}
}
