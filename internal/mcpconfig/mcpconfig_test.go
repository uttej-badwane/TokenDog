package mcpconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func setConfigPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "claude_desktop_config.json")
	t.Setenv("TD_CLAUDE_DESKTOP_CONFIG", path)
	return path
}

func TestInspectMissingFile(t *testing.T) {
	setConfigPath(t)
	state, cfg, err := Inspect()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != StateConfigMissing {
		t.Errorf("state = %s, want config-missing", state)
	}
	if cfg != nil {
		t.Errorf("cfg should be nil for missing file, got %v", cfg)
	}
}

func TestInspectEmpty(t *testing.T) {
	path := setConfigPath(t)
	if err := os.WriteFile(path, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	state, _, _ := Inspect()
	if state != StateConfigEmpty {
		t.Errorf("state = %s, want config-empty", state)
	}
}

func TestInspectOtherServers(t *testing.T) {
	path := setConfigPath(t)
	body := `{"mcpServers":{"some-other":{"command":"/bin/foo"}}}`
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	state, _, _ := Inspect()
	if state != StateOtherServers {
		t.Errorf("state = %s, want other-servers", state)
	}
}

func TestInspectInstalled(t *testing.T) {
	path := setConfigPath(t)
	body := `{"mcpServers":{"tokendog":{"command":"/usr/local/bin/td","args":["mcp"]}}}`
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	state, _, _ := Inspect()
	if state != StateInstalled {
		t.Errorf("state = %s, want installed", state)
	}
}

func TestInspectMalformed(t *testing.T) {
	path := setConfigPath(t)
	if err := os.WriteFile(path, []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	state, _, _ := Inspect()
	if state != StateMalformed {
		t.Errorf("state = %s, want malformed", state)
	}
}

func TestInstallFromMissing(t *testing.T) {
	path := setConfigPath(t)
	entry := ServerEntry{Command: "/usr/local/bin/td", Args: []string{"mcp"}}
	got, changed, err := Install(entry)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if got != path {
		t.Errorf("path = %q, want %q", got, path)
	}
	if !changed {
		t.Error("expected changed=true on first install")
	}
	state, cfg, _ := Inspect()
	if state != StateInstalled {
		t.Errorf("state = %s, want installed", state)
	}
	servers := cfg["mcpServers"].(map[string]any)
	td := servers["tokendog"].(map[string]any)
	if td["command"] != "/usr/local/bin/td" {
		t.Errorf("command = %v", td["command"])
	}
}

func TestInstallPreservesOtherServers(t *testing.T) {
	path := setConfigPath(t)
	body := `{"mcpServers":{"existing":{"command":"/x"}}, "otherKey": "preserve me"}`
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	entry := ServerEntry{Command: "/usr/local/bin/td", Args: []string{"mcp"}}
	if _, _, err := Install(entry); err != nil {
		t.Fatalf("Install: %v", err)
	}
	data, _ := os.ReadFile(path)
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	servers := cfg["mcpServers"].(map[string]any)
	if _, ok := servers["existing"]; !ok {
		t.Errorf("existing server clobbered: %v", servers)
	}
	if cfg["otherKey"] != "preserve me" {
		t.Errorf("top-level key clobbered: %v", cfg["otherKey"])
	}
}

func TestInstallIdempotent(t *testing.T) {
	setConfigPath(t)
	entry := ServerEntry{Command: "/usr/local/bin/td", Args: []string{"mcp"}}
	if _, changed, _ := Install(entry); !changed {
		t.Error("expected changed=true on first install")
	}
	if _, changed, _ := Install(entry); changed {
		t.Error("expected changed=false on second identical install")
	}
}

func TestInstallUpdatesChangedEntry(t *testing.T) {
	setConfigPath(t)
	first := ServerEntry{Command: "/old/td", Args: []string{"mcp"}}
	if _, _, err := Install(first); err != nil {
		t.Fatal(err)
	}
	second := ServerEntry{Command: "/new/td", Args: []string{"mcp"}}
	_, changed, err := Install(second)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("expected changed=true when command path changes")
	}
}

func TestInstallRefusesMalformed(t *testing.T) {
	path := setConfigPath(t)
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}
	entry := ServerEntry{Command: "/td", Args: []string{"mcp"}}
	if _, _, err := Install(entry); err == nil {
		t.Error("expected error on malformed config; got nil")
	}
	// File should not have been touched.
	data, _ := os.ReadFile(path)
	if string(data) != "not json" {
		t.Errorf("malformed file was modified: %q", data)
	}
}

func TestUninstallIdempotent(t *testing.T) {
	setConfigPath(t)
	if _, removed, _ := Uninstall(); removed {
		t.Error("uninstall on missing config should not report removed=true")
	}
}
