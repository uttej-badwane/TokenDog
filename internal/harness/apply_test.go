package harness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestApplyDupPerm(t *testing.T) {
	tmp := t.TempDir()
	settings := filepath.Join(tmp, "settings.json")
	original := `{
  "permissions": {
    "allow": [
      "Bash(git *)",
      "Read(~/docs/**)",
      "Bash(git *)"
    ]
  }
}
`
	if err := os.WriteFile(settings, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	backupDir := filepath.Join(tmp, "backups")

	fix, ok := parseFixID(fixIDDupPerm(settings, "allow", "Bash(git *)"))
	if !ok {
		t.Fatal("parseFixID failed")
	}
	applier := NewApplier(backupDir)
	if err := applier.Apply(fix); err != nil {
		t.Fatal(err)
	}
	if err := applier.Finish(); err != nil {
		t.Fatal(err)
	}

	// Duplicate removed, first occurrence and other rules kept.
	var cfg map[string]any
	data, _ := os.ReadFile(settings)
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	allow := cfg["permissions"].(map[string]any)["allow"].([]any)
	if len(allow) != 2 {
		t.Fatalf("allow = %v, want 2 entries", allow)
	}
	seen := map[string]int{}
	for _, v := range allow {
		seen[v.(string)]++
	}
	if seen["Bash(git *)"] != 1 || seen["Read(~/docs/**)"] != 1 {
		t.Errorf("wrong dedupe result: %v", allow)
	}

	// Backup is byte-identical to the pre-change original.
	entries, _ := os.ReadDir(backupDir)
	var backupData []byte
	for _, e := range entries {
		if e.Name() != "manifest.json" {
			backupData, _ = os.ReadFile(filepath.Join(backupDir, e.Name()))
		}
	}
	if string(backupData) != original {
		t.Errorf("backup not identical to original:\n%s", backupData)
	}
	// Manifest written.
	if _, err := os.Stat(filepath.Join(backupDir, "manifest.json")); err != nil {
		t.Errorf("manifest missing: %v", err)
	}

	// Re-running the same fix is a harmless no-op (already deduped).
	applier2 := NewApplier(filepath.Join(tmp, "backups2"))
	if err := applier2.Apply(fix); err != nil {
		t.Fatal(err)
	}
	data2, _ := os.ReadFile(settings)
	_ = json.Unmarshal(data2, &cfg)
	if got := len(cfg["permissions"].(map[string]any)["allow"].([]any)); got != 2 {
		t.Errorf("second run changed count to %d", got)
	}
}

func TestApplyHookExec(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no exec bit on windows")
	}
	tmp := t.TempDir()
	script := filepath.Join(tmp, "hook.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0644); err != nil {
		t.Fatal(err)
	}
	fix, ok := parseFixID(fixIDHookExec(script))
	if !ok {
		t.Fatal("parseFixID failed")
	}
	applier := NewApplier(filepath.Join(tmp, "backups"))
	if err := applier.Apply(fix); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(script)
	if info.Mode().Perm()&0111 == 0 {
		t.Errorf("exec bit not set: %v", info.Mode())
	}
}

func TestAutoFixesDedup(t *testing.T) {
	r := &Report{Findings: []Finding{
		{AutoFixable: true, FixID: fixIDHookExec("/a.sh")},
		{AutoFixable: true, FixID: fixIDHookExec("/a.sh")}, // dup id
		{AutoFixable: false, FixID: "ignored"},
		{AutoFixable: true, FixID: fixIDDupPerm("/s.json", "allow", "Bash(*)")},
	}}
	fixes := AutoFixes(r)
	if len(fixes) != 2 {
		t.Fatalf("AutoFixes = %d, want 2 (%+v)", len(fixes), fixes)
	}
}
