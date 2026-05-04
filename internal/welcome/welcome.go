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
	renderLogo(colors)
	fmt.Println()
	fmt.Println(colors.gray("  Token-optimized CLI proxy for AI coding assistants  ") + colors.dim(version))
	fmt.Println()

	renderSetup(colors)
	renderQuickStart(colors)
	renderCommands(colors)

	fmt.Println()
	fmt.Println(colors.gray("  Documentation:  https://github.com/uttej-badwane/TokenDog"))
	fmt.Println()
}

// figlet rows for "TOKENDOG" in ANSI Shadow style. Each row is 69 runes wide
// with consistent letter-column boundaries:
//
//	T=[0:9]  O=[9:18]  K=[18:26]  E=[26:34]  N=[34:44]
//	D=[44:52]  O=[52:61]  G=[61:69]
var figletRows = []string{
	"████████╗ ██████╗ ██╗  ██╗███████╗███╗   ██╗██████╗  ██████╗  ██████╗",
	"╚══██╔══╝██╔═══██╗██║ ██╔╝██╔════╝████╗  ██║██╔══██╗██╔═══██╗██╔════╝",
	"   ██║   ██║   ██║█████╔╝ █████╗  ██╔██╗ ██║██║  ██║██║   ██║██║  ███╗",
	"   ██║   ██║   ██║██╔═██╗ ██╔══╝  ██║╚██╗██║██║  ██║██║   ██║██║   ██║",
	"   ██║   ╚██████╔╝██║  ██╗███████╗██║ ╚████║██████╔╝╚██████╔╝╚██████╔╝",
	"   ╚═╝    ╚═════╝ ╚═╝  ╚═╝╚══════╝╚═╝  ╚═══╝╚═════╝  ╚═════╝  ╚═════╝",
}

// renderLogo prints the TOKENDOG figlet in bold white — same look across
// the install caveats, README, and welcome screen.
func renderLogo(c *colors) {
	for _, line := range figletRows {
		fmt.Println("  " + c.bold(c.white(line)))
	}
}

func renderSetup(c *colors) {
	binary, _ := os.Executable()
	if binary == "" {
		binary = "(detected at runtime)"
	}

	pre := detectClaudeHooks()

	fmt.Println(c.bold("  Setup"))
	fmt.Println()
	fmt.Println("    " + c.green("●") + " Binary installed                  " + c.gray(binary))
	statusLine(c, "PreToolUse / Bash", pre)
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
		{"td discover", "Find unrewritten commands in your Claude history"},
		{"td git status", "Filtered git status (manual)"},
		{"td gh pr list", "Compact GitHub PR/issue/run tables"},
		{"td pytest", "Test runners — strict-mode summary, verbatim on failure"},
		{"td aws / gcloud / az", "Cloud CLI JSON/table/YAML compaction"},
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
		PreToolUse []claudeHookGroup `json:"PreToolUse"`
	} `json:"hooks"`
}

func detectClaudeHooks() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	data, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		return false
	}
	var s claudeSettings
	if err := json.Unmarshal(data, &s); err != nil {
		return false
	}
	for _, g := range s.Hooks.PreToolUse {
		if g.Matcher != "Bash" {
			continue
		}
		for _, h := range g.Hooks {
			if strings.Contains(h.Command, "td hook claude") {
				return true
			}
		}
	}
	return false
}
