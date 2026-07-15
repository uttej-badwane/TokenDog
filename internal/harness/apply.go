package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Fix IDs are the wire format between the audit and `td harness apply`:
// kind|path|details. Only findings whose fix is mechanical and reversible
// ever get one — everything else stays report-only.

func fixIDDupPerm(path, list, rule string) string {
	return strings.Join([]string{"dup-perm", path, list, rule}, "|")
}

func fixIDHookExec(path string) string {
	return "hook-exec|" + path
}

// Fix is one approved-and-applicable change.
type Fix struct {
	ID          string
	Kind        string // "dup-perm" | "hook-exec"
	Path        string
	Description string

	list, rule string // dup-perm details
}

// AutoFixes extracts the deduplicated list of applicable fixes from a
// report, in report (severity) order.
func AutoFixes(r *Report) []Fix {
	var out []Fix
	seen := map[string]bool{}
	for _, f := range r.Findings {
		if !f.AutoFixable || f.FixID == "" || seen[f.FixID] {
			continue
		}
		seen[f.FixID] = true
		if fix, ok := parseFixID(f.FixID); ok {
			out = append(out, fix)
		}
	}
	return out
}

func parseFixID(id string) (Fix, bool) {
	kind, rest, _ := strings.Cut(id, "|")
	switch kind {
	case "dup-perm":
		// path|list|rule — rule is last so a rule containing '|' survives.
		parts := strings.SplitN(rest, "|", 3)
		if len(parts) != 3 {
			return Fix{}, false
		}
		return Fix{
			ID: id, Kind: kind, Path: parts[0], list: parts[1], rule: parts[2],
			Description: fmt.Sprintf("remove duplicate rule %q from permissions.%s in %s", parts[2], parts[1], Tildify(parts[0])),
		}, true
	case "hook-exec":
		if rest == "" {
			return Fix{}, false
		}
		return Fix{
			ID: id, Kind: kind, Path: rest,
			Description: "chmod +x " + Tildify(rest),
		}, true
	}
	return Fix{}, false
}

// DefaultBackupDir is where `td harness apply` stashes pre-change copies:
// ~/.config/tokendog/harness-backups/<timestamp>/. Overridable via
// TD_HARNESS_BACKUP_DIR for tests.
func DefaultBackupDir(now time.Time) (string, error) {
	if override := os.Getenv("TD_HARNESS_BACKUP_DIR"); override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "tokendog", "harness-backups", now.Format("20060102-150405")), nil
}

// Applier executes fixes with per-file backups. Every file is backed up
// once (before its first mutation); Finish writes a manifest mapping
// each backup to its original path so a restore is a plain copy back.
type Applier struct {
	BackupDir string

	backedUp map[string]bool
	manifest []manifestEntry
	seq      int
}

type manifestEntry struct {
	Backup   string `json:"backup,omitempty"`
	Original string `json:"original"`
	Action   string `json:"action"`
}

func NewApplier(backupDir string) *Applier {
	return &Applier{BackupDir: backupDir, backedUp: map[string]bool{}}
}

// Apply executes one fix. Content-mutating fixes back the file up first;
// chmod fixes are reversible by nature and only recorded in the manifest.
func (a *Applier) Apply(f Fix) error {
	switch f.Kind {
	case "dup-perm":
		if err := a.backup(f.Path); err != nil {
			return err
		}
		return applyDupPerm(f)
	case "hook-exec":
		info, err := os.Stat(f.Path)
		if err != nil {
			return err
		}
		if err := os.Chmod(f.Path, info.Mode().Perm()|0111); err != nil {
			return err
		}
		a.manifest = append(a.manifest, manifestEntry{Original: f.Path, Action: "chmod +x (revert: chmod -x)"})
		return nil
	}
	return fmt.Errorf("unknown fix kind %q", f.Kind)
}

// Finish writes the manifest. No-op when nothing was applied.
func (a *Applier) Finish() error {
	if len(a.manifest) == 0 {
		return nil
	}
	data, err := json.MarshalIndent(a.manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(a.BackupDir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(a.BackupDir, "manifest.json"), append(data, '\n'), 0644)
}

func (a *Applier) backup(path string) error {
	if a.backedUp[path] {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(a.BackupDir, 0755); err != nil {
		return err
	}
	a.seq++
	name := fmt.Sprintf("%02d-%s", a.seq, sanitizeForFilename(path))
	if err := os.WriteFile(filepath.Join(a.BackupDir, name), data, 0600); err != nil {
		return err
	}
	a.backedUp[path] = true
	a.manifest = append(a.manifest, manifestEntry{Backup: name, Original: path, Action: "edited (revert: copy backup over original)"})
	return nil
}

func sanitizeForFilename(path string) string {
	return strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_").Replace(strings.Trim(path, "/\\"))
}

// applyDupPerm removes later duplicates of one rule inside one
// permissions list, keeping the first occurrence. Note: the map-based
// rewrite re-indents and alphabetizes keys — same trade-off the rest of
// the codebase already makes for settings edits (cmd/setup.go), and the
// backup preserves the exact original.
func applyDupPerm(f Fix) error {
	data, err := os.ReadFile(f.Path)
	if err != nil {
		return err
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("%s changed since the audit and no longer parses: %w", f.Path, err)
	}
	perms, _ := cfg["permissions"].(map[string]any)
	raw, _ := perms[f.list].([]any)
	if raw == nil {
		return fmt.Errorf("permissions.%s no longer present in %s", f.list, f.Path)
	}
	kept := make([]any, 0, len(raw))
	found := false
	for _, v := range raw {
		if s, ok := v.(string); ok && s == f.rule {
			if found {
				continue
			}
			found = true
		}
		kept = append(kept, v)
	}
	perms[f.list] = kept
	return writeAtomicJSON(f.Path, cfg)
}

// writeAtomicJSON mirrors mcpconfig.writeAtomic: temp file + rename so a
// crash can't leave a half-written settings file.
func writeAtomicJSON(path string, cfg map[string]any) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
