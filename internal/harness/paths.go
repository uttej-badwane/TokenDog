package harness

import (
	"os"
	"path/filepath"
	"strings"
)

// ClaudeHome returns the Claude Code config directory, ~/.claude by
// default. Overridable via TD_CLAUDE_HOME for nonstandard installs and
// for testing (same pattern as TD_CLAUDE_DESKTOP_CONFIG in mcpconfig).
func ClaudeHome() (string, error) {
	if override := os.Getenv("TD_CLAUDE_HOME"); override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude"), nil
}

// ClaudeJSONPath returns the path of Claude Code's state file,
// ~/.claude.json by default. Overridable via TD_CLAUDE_JSON.
func ClaudeJSONPath() (string, error) {
	if override := os.Getenv("TD_CLAUDE_JSON"); override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude.json"), nil
}

// FindProjectRoot walks upward from start looking for the active
// project: the first directory containing a .claude/ dir wins, else the
// first containing .git. The walk stops at the user's home directory and
// the filesystem root — $HOME itself is never a project (its .claude is
// the user scope), so running from ~ yields no project scope. Returns ""
// when nothing qualifies.
func FindProjectRoot(start string) string {
	home, _ := os.UserHomeDir()
	dir, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for {
		if home != "" && dir == home {
			return ""
		}
		if isDir(filepath.Join(dir, ".claude")) || isDir(filepath.Join(dir, ".git")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// expandHome resolves a leading ~/ against the real home directory.
// Returns the input unchanged when there's nothing to expand.
func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}
	return path
}

// Tildify replaces the home-dir prefix with ~ for compact display.
func Tildify(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+string(filepath.Separator)) {
		return "~" + path[len(home):]
	}
	return path
}
