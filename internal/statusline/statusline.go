// Package statusline renders TokenDog's own status line from the JSON Claude
// Code pipes to a statusLine command on stdin. It is self-contained: no external
// status-line tool is invoked, and git state is read directly from .git rather
// than shelling out, so a render is a couple of file reads and no subprocess.
package statusline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Payload is the subset of Claude Code's statusLine stdin schema the renderer
// uses. See code.claude.com/docs/en/statusline for the full structure.
type Payload struct {
	Dir         string  // workspace.current_dir, falling back to cwd
	Model       string  // model.display_name
	Effort      string  // effort.level
	ContextPct  float64 // context_window.used_percentage
	Exceeds200k bool    // exceeds_200k_tokens
	CostUSD     float64 // cost.total_cost_usd
}

// ANSI colors. Terracotta approximates TokenDog's brand accent; context usage
// is traffic-lit by fullness.
const (
	reset      = "\x1b[0m"
	bold       = "\x1b[1m"
	dim        = "\x1b[2m"
	terracotta = "\x1b[38;5;173m"
	green      = "\x1b[38;5;71m"
	yellow     = "\x1b[38;5;179m"
	red        = "\x1b[38;5;167m"
)

// Render returns a single status line, e.g. "TokenDog (main)  Opus 4.8  8% ctx  $2.10".
// Color is emitted unless NO_COLOR is set.
func Render(p Payload) string {
	color := colorEnabled()
	var seg []string

	if dir := dirLabel(p.Dir); dir != "" {
		seg = append(seg, paint(color, bold, dir))
	}
	if b := gitBranch(p.Dir); b != "" {
		seg = append(seg, paint(color, dim, "("+b+")"))
	}
	if p.Model != "" {
		m := p.Model
		if p.Effort != "" {
			m += " " + p.Effort
		}
		seg = append(seg, paint(color, terracotta, m))
	}
	if p.ContextPct > 0 || p.Exceeds200k {
		seg = append(seg, paint(color, ctxColor(p.ContextPct, p.Exceeds200k),
			fmt.Sprintf("%.0f%% ctx", p.ContextPct)))
	}
	seg = append(seg, paint(color, green, fmt.Sprintf("$%.2f", p.CostUSD)))

	return strings.Join(seg, "  ")
}

func paint(enabled bool, code, s string) string {
	if !enabled {
		return s
	}
	return code + s + reset
}

func colorEnabled() bool {
	_, noColor := os.LookupEnv("NO_COLOR")
	return !noColor
}

func ctxColor(pct float64, exceeds bool) string {
	switch {
	case exceeds || pct >= 80:
		return red
	case pct >= 50:
		return yellow
	default:
		return green
	}
}

// dirLabel is the trailing path component, or "~" for the home directory.
func dirLabel(dir string) string {
	if dir == "" {
		return ""
	}
	if home, err := os.UserHomeDir(); err == nil && dir == home {
		return "~"
	}
	return filepath.Base(dir)
}

// gitBranch resolves the current branch by reading .git/HEAD from dir upward,
// with no subprocess. Returns a short SHA for a detached HEAD, or "" if dir is
// not in a git repo.
func gitBranch(dir string) string {
	if dir == "" {
		return ""
	}
	d := dir
	for i := 0; i < 40; i++ {
		gitPath := filepath.Join(d, ".git")
		if info, err := os.Stat(gitPath); err == nil {
			if info.IsDir() {
				return readHEAD(filepath.Join(gitPath, "HEAD"))
			}
			// Worktree/submodule: .git is a file "gitdir: <path>".
			if data, err := os.ReadFile(gitPath); err == nil {
				gd := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(data)), "gitdir:"))
				if gd != "" {
					return readHEAD(filepath.Join(gd, "HEAD"))
				}
			}
			return ""
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	return ""
}

func readHEAD(headPath string) string {
	data, err := os.ReadFile(headPath)
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(data))
	if ref := strings.TrimPrefix(s, "ref: refs/heads/"); ref != s {
		return ref
	}
	if len(s) >= 7 { // detached HEAD: short SHA
		return s[:7]
	}
	return ""
}
