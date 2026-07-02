package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// statusLine wiring for `td setup` / `td unsetup`.
//
// Claude Code pushes its own session cost (cost.total_cost_usd, the /cost
// figure) to whatever single command is registered as `statusLine` in
// settings.json. Setup points that at `td statusline`, which renders
// TokenDog's own status line and records the cost. Any prior statusLine is
// backed up so unsetup can restore it verbatim.

const tdStatuslineMarker = "td statusline"

func claudeSettingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

func statuslineBackupPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "tokendog", "statusline-backup.json"), nil
}

// writeSettingsAtomic marshals settings and replaces path via temp+rename so a
// partial write can't corrupt the file.
func writeSettingsAtomic(path string, settings map[string]any) error {
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(out, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// installStatusLine points settings.json's statusLine at `td statusline`,
// backing up any prior statusLine so unsetup can restore it. Idempotent.
func installStatusLine() (string, error) {
	path, err := claudeSettingsPath()
	if err != nil {
		return "", err
	}

	settings := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			// Don't clobber a file we can't parse.
			return "", fmt.Errorf("settings.json is not valid JSON; not modifying it: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}

	cur, _ := settings["statusLine"].(map[string]any)
	curCmd, _ := cur["command"].(string)

	if strings.Contains(curCmd, tdStatuslineMarker) {
		return "statusLine already routes through td (skipping)", nil
	}

	// Preserve the user's other statusLine fields (type, padding,
	// refreshInterval, …); only the command changes to td statusline.
	newSL := map[string]any{}
	for k, v := range cur {
		newSL[k] = v
	}
	newSL["type"] = "command"
	newSL["command"] = "td statusline"

	msg := "registered td statusline"
	if strings.TrimSpace(curCmd) != "" {
		// Back up the prior statusLine so unsetup can restore it exactly.
		if bp, err := statuslineBackupPath(); err == nil {
			if _, statErr := os.Stat(bp); os.IsNotExist(statErr) { // don't overwrite a prior backup
				if b, mErr := json.MarshalIndent(cur, "", "  "); mErr == nil {
					_ = os.MkdirAll(filepath.Dir(bp), 0o755)
					_ = os.WriteFile(bp, b, 0o644)
				}
			}
		}
		msg = "registered td statusline (backed up your prior statusLine)"
	}

	settings["statusLine"] = newSL
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := writeSettingsAtomic(path, settings); err != nil {
		return "", err
	}
	return msg, nil
}

// removeStatusLine reverses installStatusLine: it restores the backed-up
// original statusLine, or removes the entry entirely if td added it as the sole
// provider. A statusLine the user has since changed off td is left alone.
func removeStatusLine() (string, error) {
	path, err := claudeSettingsPath()
	if err != nil {
		return "", err
	}
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

	cur, _ := settings["statusLine"].(map[string]any)
	curCmd, _ := cur["command"].(string)
	if !strings.Contains(curCmd, tdStatuslineMarker) {
		return "statusLine no longer routes through td (leaving as-is)", nil
	}

	bp, _ := statuslineBackupPath()
	if bp != "" {
		if b, err := os.ReadFile(bp); err == nil {
			var orig map[string]any
			if json.Unmarshal(b, &orig) == nil {
				settings["statusLine"] = orig
				if err := writeSettingsAtomic(path, settings); err != nil {
					return "", err
				}
				_ = os.Remove(bp)
				return "restored your original statusLine", nil
			}
		}
	}

	// No backup ⇒ td was the sole provider; drop the entry.
	delete(settings, "statusLine")
	if err := writeSettingsAtomic(path, settings); err != nil {
		return "", err
	}
	return "removed td statusLine entry", nil
}
