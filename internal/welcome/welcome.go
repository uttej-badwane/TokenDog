package welcome

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Render prints the welcome screen to stdout. It auto-detects terminal
// color support and Claude Code hook configuration.
func Render(version string) {
	colors := newColors()

	fmt.Println()
	fmt.Println(colors.bold(colors.cyan("  TokenDog ")) + colors.gray(version))
	fmt.Println(colors.gray("  Token-optimized CLI proxy for AI coding assistants"))
	fmt.Println()

	renderSetup(colors)
	renderQuickStart(colors)
	renderCommands(colors)

	fmt.Println()
	fmt.Println(colors.gray("  Documentation:  https://github.com/uttej-badwane/TokenDog"))
	fmt.Println()
}

func renderSetup(c *colors) {
	binary, _ := os.Executable()
	if binary == "" {
		binary = "(detected at runtime)"
	}

	pre, fetch, glob, grep, search := detectClaudeHooks()

	fmt.Println(c.bold("  Setup"))
	fmt.Println()
	fmt.Println("    " + c.green("●") + " Binary installed                  " + c.gray(binary))
	statusLine(c, "PreToolUse / Bash", pre)
	statusLine(c, "PostToolUse / WebFetch", fetch)
	statusLine(c, "PostToolUse / Glob", glob)
	statusLine(c, "PostToolUse / Grep", grep)
	statusLine(c, "PostToolUse / WebSearch", search)
	fmt.Println()
}

func statusLine(c *colors, label string, ok bool) {
	icon := c.yellow("○")
	state := c.yellow("not configured")
	if ok {
		icon = c.green("●")
		state = c.green("configured")
	}
	pad := strings.Repeat(" ", max(0, 33-len(label)))
	fmt.Printf("    %s %s%s%s\n", icon, label, pad, state)
}

func renderQuickStart(c *colors) {
	fmt.Println(c.bold("  Quick start"))
	fmt.Println()
	fmt.Println("    Add the following to " + c.cyan("~/.claude/settings.json") + ":")
	fmt.Println()
	hookSnippet := `    {
      "hooks": {
        "PreToolUse": [
          {"matcher":"Bash","hooks":[{"type":"command","command":"td hook claude"}]}
        ],
        "PostToolUse": [
          {"matcher":"WebFetch", "hooks":[{"type":"command","command":"td pipe webfetch"}]},
          {"matcher":"Glob",     "hooks":[{"type":"command","command":"td pipe glob"}]},
          {"matcher":"Grep",     "hooks":[{"type":"command","command":"td pipe grep"}]},
          {"matcher":"WebSearch","hooks":[{"type":"command","command":"td pipe websearch"}]}
        ]
      }
    }`
	for _, line := range strings.Split(hookSnippet, "\n") {
		fmt.Println(c.dim(line))
	}
	fmt.Println()
}

func renderCommands(c *colors) {
	fmt.Println(c.bold("  Commands"))
	fmt.Println()
	rows := [][2]string{
		{"td gain", "View token savings summary"},
		{"td gain --history", "Recent commands with savings"},
		{"td git status", "Filtered git status (manual)"},
		{"td rewrite <cmd>", "Debug: see how a command is rewritten"},
		{"td --help", "All commands"},
	}
	for _, r := range rows {
		fmt.Printf("    %s%s%s\n", c.cyan(r[0]), strings.Repeat(" ", max(2, 22-len(r[0]))), c.gray(r[1]))
	}
}

// MarkInitialized writes a marker so the auto-trigger doesn't fire again.
func MarkInitialized() error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(
		filepath.Join(dir, "initialized"),
		[]byte(time.Now().UTC().Format(time.RFC3339)),
		0644,
	)
}

// IsFirstRun reports whether the marker file is missing.
func IsFirstRun() bool {
	dir, err := configDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(dir, "initialized"))
	return os.IsNotExist(err)
}

func configDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "tokendog"), nil
}

// --- Claude Code settings detection ---

type claudeHookEntry struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

type claudeHookGroup struct {
	Matcher string            `json:"matcher"`
	Hooks   []claudeHookEntry `json:"hooks"`
}

type claudeSettings struct {
	Hooks struct {
		PreToolUse  []claudeHookGroup `json:"PreToolUse"`
		PostToolUse []claudeHookGroup `json:"PostToolUse"`
	} `json:"hooks"`
}

func detectClaudeHooks() (pre, fetch, glob, grep, search bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	data, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		return
	}
	var s claudeSettings
	if err := json.Unmarshal(data, &s); err != nil {
		return
	}
	for _, g := range s.Hooks.PreToolUse {
		if g.Matcher != "Bash" {
			continue
		}
		for _, h := range g.Hooks {
			if strings.Contains(h.Command, "td hook claude") {
				pre = true
			}
		}
	}
	for _, g := range s.Hooks.PostToolUse {
		for _, h := range g.Hooks {
			switch g.Matcher {
			case "WebFetch":
				if strings.Contains(h.Command, "td pipe webfetch") {
					fetch = true
				}
			case "Glob":
				if strings.Contains(h.Command, "td pipe glob") {
					glob = true
				}
			case "Grep":
				if strings.Contains(h.Command, "td pipe grep") {
					grep = true
				}
			case "WebSearch":
				if strings.Contains(h.Command, "td pipe websearch") {
					search = true
				}
			}
		}
	}
	return
}
