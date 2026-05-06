// Package mcpconfig manages Claude Desktop's claude_desktop_config.json —
// the file users must edit to register MCP servers. This package centralizes
// the OS-specific path detection, the read/merge/write cycle, and the
// state-machine that distinguishes "developer mode never enabled" from
// "ready to install" from "tokendog already present".
//
// Why a separate package: cross-platform path detection + atomic file
// writes + JSON merge logic is enough surface to test in isolation. The
// cmd-layer wrapper (`td mcp install/config/doctor`) is then a thin shell.
package mcpconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

// ServerName is the key used in mcpServers for tokendog. Stable so
// upgrades find the existing entry rather than creating a duplicate.
const ServerName = "tokendog"

// State enumerates the user-visible states of Claude Desktop's MCP
// configuration. The doctor command renders each one with a specific
// remediation message.
type State int

const (
	// StateConfigMissing means claude_desktop_config.json does not exist at
	// the expected path. Almost always means the user hasn't enabled
	// Developer Mode yet (Help > Troubleshooting > Enable Developer Mode
	// in Claude Desktop, which seeds the file on first save).
	StateConfigMissing State = iota
	// StateConfigEmpty means the file exists but is empty / has no
	// mcpServers key. Developer Mode is enabled but no servers configured
	// yet — `td mcp install` is the right next step.
	StateConfigEmpty
	// StateOtherServers means mcpServers exists with at least one entry,
	// but no tokendog. Install will add tokendog without touching others.
	StateOtherServers
	// StateInstalled means tokendog is already in mcpServers. Doctor
	// additionally validates the binary path still exists.
	StateInstalled
	// StateMalformed means the file exists but isn't valid JSON. Install
	// refuses to touch it; user must fix manually.
	StateMalformed
	// StateUnsupportedOS — we don't know where Claude Desktop stores its
	// config on this OS. Currently catches anything that isn't darwin,
	// windows, or linux.
	StateUnsupportedOS
)

func (s State) String() string {
	switch s {
	case StateConfigMissing:
		return "config-missing"
	case StateConfigEmpty:
		return "config-empty"
	case StateOtherServers:
		return "other-servers"
	case StateInstalled:
		return "installed"
	case StateMalformed:
		return "malformed"
	case StateUnsupportedOS:
		return "unsupported-os"
	}
	return "unknown"
}

// ConfigPath returns the platform-specific path where Claude Desktop
// stores claude_desktop_config.json. Returns an error on unsupported OSes.
//
// macOS:   ~/Library/Application Support/Claude/claude_desktop_config.json
// Windows: %APPDATA%\Claude\claude_desktop_config.json
// Linux:   ~/.config/Claude/claude_desktop_config.json
//
// The path is overridable via TD_CLAUDE_DESKTOP_CONFIG for users with
// nonstandard installs and for testing.
func ConfigPath() (string, error) {
	if override := os.Getenv("TD_CLAUDE_DESKTOP_CONFIG"); override != "" {
		return override, nil
	}
	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json"), nil
	case "windows":
		appdata := os.Getenv("APPDATA")
		if appdata == "" {
			return "", fmt.Errorf("APPDATA env var not set")
		}
		return filepath.Join(appdata, "Claude", "claude_desktop_config.json"), nil
	case "linux":
		// Linux convention. Anthropic doesn't ship an official Linux build
		// at the time of this writing, but community wrappers / Wine
		// installs put it here.
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".config", "Claude", "claude_desktop_config.json"), nil
	}
	return "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
}

// ServerEntry is the value type under mcpServers["tokendog"]. The shape
// matches Claude Desktop's expected schema.
type ServerEntry struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// Inspect reads the config file and returns its current State plus the
// parsed config (nil if unreadable/malformed/missing). A non-nil error
// means we couldn't even classify the state — actual file-read errors
// for missing files are NOT returned; they map to StateConfigMissing.
func Inspect() (State, map[string]any, error) {
	path, err := ConfigPath()
	if err != nil {
		return StateUnsupportedOS, nil, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return StateConfigMissing, nil, nil
	}
	if err != nil {
		return StateMalformed, nil, err
	}
	if len(data) == 0 || isWhitespaceOnly(data) {
		return StateConfigEmpty, map[string]any{}, nil
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		return StateMalformed, nil, err
	}
	servers, _ := cfg["mcpServers"].(map[string]any)
	if len(servers) == 0 {
		return StateConfigEmpty, cfg, nil
	}
	if _, ok := servers[ServerName]; ok {
		return StateInstalled, cfg, nil
	}
	return StateOtherServers, cfg, nil
}

// Install writes a tokendog ServerEntry into mcpServers, preserving any
// existing entries. Returns the path that was written and a boolean
// indicating whether the file actually changed (false = idempotent re-run
// with identical entry).
//
// On StateMalformed or StateUnsupportedOS we refuse — we don't want to
// clobber a file we couldn't parse. Caller (doctor / install command)
// should surface the State to the user.
func Install(entry ServerEntry) (path string, changed bool, err error) {
	state, cfg, _ := Inspect()
	if state == StateMalformed || state == StateUnsupportedOS {
		return "", false, fmt.Errorf("cannot install: config is in state %s", state)
	}
	path, err = ConfigPath()
	if err != nil {
		return "", false, err
	}
	if cfg == nil {
		cfg = map[string]any{}
	}
	servers, _ := cfg["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}

	// Compare current entry to proposed; skip the write if identical so a
	// re-run is a true no-op (no mtime bump that might trigger Claude
	// Desktop to re-load).
	if existing, ok := servers[ServerName]; ok {
		if entriesEqual(existing, entry) {
			return path, false, nil
		}
	}

	servers[ServerName] = entry
	cfg["mcpServers"] = servers

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return path, false, err
	}
	if err := writeAtomic(path, cfg); err != nil {
		return path, false, err
	}
	return path, true, nil
}

// Uninstall removes the tokendog entry. Idempotent — no-op if not present.
func Uninstall() (path string, removed bool, err error) {
	state, cfg, _ := Inspect()
	if state != StateInstalled {
		path, _ = ConfigPath()
		return path, false, nil
	}
	path, err = ConfigPath()
	if err != nil {
		return "", false, err
	}
	servers, _ := cfg["mcpServers"].(map[string]any)
	delete(servers, ServerName)
	cfg["mcpServers"] = servers
	return path, true, writeAtomic(path, cfg)
}

// writeAtomic writes the config via temp-file + rename so a crash
// mid-write can't leave Claude Desktop with a half-written config.
func writeAtomic(path string, cfg map[string]any) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	// Trailing newline matches what most editors produce — quiet diff if
	// the user has the file open.
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func entriesEqual(existing any, proposed ServerEntry) bool {
	m, ok := existing.(map[string]any)
	if !ok {
		return false
	}
	if cmd, _ := m["command"].(string); cmd != proposed.Command {
		return false
	}
	// Args comparison: existing is []any, proposed is []string. Stringify.
	existingArgs, _ := m["args"].([]any)
	if len(existingArgs) != len(proposed.Args) {
		return false
	}
	for i, a := range existingArgs {
		s, _ := a.(string)
		if s != proposed.Args[i] {
			return false
		}
	}
	return true
}

func isWhitespaceOnly(b []byte) bool {
	for _, c := range b {
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			return false
		}
	}
	return true
}
