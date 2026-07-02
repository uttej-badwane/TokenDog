package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func readStatusLine(t *testing.T, home string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var s map[string]any
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	sl, _ := s["statusLine"].(map[string]any)
	return sl
}

func TestInstallStatusLineReplacesAndRestores(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A pre-existing custom statusLine with an extra field to preserve.
	orig := `{"statusLine":{"type":"command","command":"my-statusline --foo","refreshInterval":10},"model":"opus"}`
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := installStatusLine(); err != nil {
		t.Fatalf("install: %v", err)
	}
	sl := readStatusLine(t, home)
	if got := sl["command"].(string); got != "td statusline" {
		t.Errorf("command = %q, want 'td statusline'", got)
	}
	if sl["refreshInterval"].(float64) != 10 {
		t.Error("refreshInterval not preserved")
	}

	// Idempotent: a second install is a no-op.
	if _, err := installStatusLine(); err != nil {
		t.Fatalf("install #2: %v", err)
	}
	if got := readStatusLine(t, home)["command"].(string); got != "td statusline" {
		t.Errorf("second install changed command to %q", got)
	}

	// Unsetup restores the prior statusLine verbatim.
	if _, err := removeStatusLine(); err != nil {
		t.Fatalf("remove: %v", err)
	}
	sl = readStatusLine(t, home)
	if got := sl["command"].(string); got != "my-statusline --foo" {
		t.Errorf("restored command = %q, want original", got)
	}
	if sl["refreshInterval"].(float64) != 10 {
		t.Error("refreshInterval lost on restore")
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "tokendog", "statusline-backup.json")); !os.IsNotExist(err) {
		t.Error("backup should be removed after restore")
	}
}

func TestInstallStatusLineSoleProvider(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	// No prior statusLine at all.
	if err := os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := installStatusLine(); err != nil {
		t.Fatalf("install: %v", err)
	}
	if got := readStatusLine(t, home)["command"].(string); got != "td statusline" {
		t.Errorf("sole-provider command = %q, want 'td statusline'", got)
	}

	// With no backup, unsetup drops the entry entirely.
	if _, err := removeStatusLine(); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if sl := readStatusLine(t, home); sl != nil {
		t.Errorf("statusLine should be removed, got %v", sl)
	}
}
