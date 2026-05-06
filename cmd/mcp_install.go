package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"tokendog/internal/mcpconfig"
)

func init() {
	mcpCmd.AddCommand(mcpInstallCmd)
	mcpCmd.AddCommand(mcpUninstallCmd)
	mcpCmd.AddCommand(mcpConfigCmd)
	mcpCmd.AddCommand(mcpDoctorCmd)
}

var mcpInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Wire TokenDog into Claude Desktop's claude_desktop_config.json",
	Long: `Adds tokendog as an MCP server in Claude Desktop's config file.

Before running this, you need to enable Developer Mode in Claude Desktop:
  Help > Troubleshooting > Enable Developer Mode

Then run ` + "`td mcp install`" + ` and restart Claude Desktop. The "tokendog"
server will appear in Settings > Developer.

If the config already contains other MCP servers, this command leaves
them alone — only the tokendog entry is added or updated.`,
	RunE: runMCPInstall,
}

var mcpUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove TokenDog from Claude Desktop's MCP server list",
	RunE:  runMCPUninstall,
}

var mcpConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Print the JSON snippet to paste into any MCP-aware client (Cursor, Cline, etc.)",
	Long: `Prints the tokendog server entry as JSON. Useful for clients that don't
have a Claude-Desktop-style config (Cursor, Cline, Continue, etc.) — you
copy the snippet and paste it into your client's MCP config UI.

For Claude Desktop specifically, prefer ` + "`td mcp install`" + ` which writes the
config atomically.`,
	RunE: runMCPConfig,
}

var mcpDoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose Claude Desktop MCP wiring (config presence, binary path, server response)",
	Long: `Checks each step of the Claude Desktop MCP setup chain and reports the
exact remediation when something is off:

  1. Is the config file at the expected path? If not, you probably haven't
     enabled Developer Mode yet (Help > Troubleshooting > Enable Developer
     Mode).
  2. Does it contain a tokendog entry? If not, run ` + "`td mcp install`" + `.
  3. Does the binary at the configured path still exist? Catches the case
     where you upgraded td and the path moved.
  4. Does ` + "`td mcp`" + ` respond to a JSON-RPC initialize? Catches PATH issues
     in Claude Desktop's launcher environment.

Each step prints ✓ or ✗ with a one-line fix.`,
	RunE: runMCPDoctor,
}

func runMCPInstall(_ *cobra.Command, _ []string) error {
	state, _, err := mcpconfig.Inspect()
	if err != nil && state != mcpconfig.StateMalformed {
		return err
	}
	switch state {
	case mcpconfig.StateUnsupportedOS:
		return fmt.Errorf("Claude Desktop config path is unknown for your OS — set TD_CLAUDE_DESKTOP_CONFIG to its full path and re-run")
	case mcpconfig.StateMalformed:
		path, _ := mcpconfig.ConfigPath()
		return fmt.Errorf("config at %s is not valid JSON; fix it manually before running install", path)
	case mcpconfig.StateConfigMissing:
		fmt.Fprintln(os.Stderr, "Claude Desktop config is missing — this usually means Developer Mode hasn't been enabled yet.")
		fmt.Fprintln(os.Stderr, "  1. Open Claude Desktop")
		fmt.Fprintln(os.Stderr, "  2. Help > Troubleshooting > Enable Developer Mode")
		fmt.Fprintln(os.Stderr, "  3. Open Settings > Developer once to seed the config file")
		fmt.Fprintln(os.Stderr, "  4. Re-run `td mcp install`")
		return fmt.Errorf("config not found; enable Developer Mode first")
	}

	entry, err := buildEntry()
	if err != nil {
		return err
	}
	path, changed, err := mcpconfig.Install(entry)
	if err != nil {
		return err
	}
	if !changed {
		fmt.Printf("✓ tokendog already installed at %s — no changes needed\n", path)
		return nil
	}
	fmt.Printf("✓ tokendog installed at %s\n", path)
	fmt.Printf("  → Restart Claude Desktop to load the new server.\n")
	return nil
}

func runMCPUninstall(_ *cobra.Command, _ []string) error {
	path, removed, err := mcpconfig.Uninstall()
	if err != nil {
		return err
	}
	if !removed {
		fmt.Printf("tokendog was not installed in %s\n", path)
		return nil
	}
	fmt.Printf("✓ tokendog removed from %s\n", path)
	fmt.Printf("  → Restart Claude Desktop to drop the server.\n")
	return nil
}

func runMCPConfig(_ *cobra.Command, _ []string) error {
	entry, err := buildEntry()
	if err != nil {
		return err
	}
	snippet := map[string]any{
		"mcpServers": map[string]any{
			mcpconfig.ServerName: entry,
		},
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(snippet)
}

func runMCPDoctor(_ *cobra.Command, _ []string) error {
	state, cfg, _ := mcpconfig.Inspect()
	path, _ := mcpconfig.ConfigPath()

	switch state {
	case mcpconfig.StateUnsupportedOS:
		fmt.Println("✗ Unsupported OS — TokenDog doesn't know where Claude Desktop stores its config here.")
		fmt.Println("  Fix: set TD_CLAUDE_DESKTOP_CONFIG to the full config path.")
		return nil
	case mcpconfig.StateMalformed:
		fmt.Printf("✗ Config at %s is not valid JSON.\n", path)
		fmt.Println("  Fix: open it in an editor, repair the syntax, and re-run.")
		return nil
	case mcpconfig.StateConfigMissing:
		fmt.Printf("✗ Config not found at %s.\n", path)
		fmt.Println("  This usually means Developer Mode is not enabled in Claude Desktop yet.")
		fmt.Println("  Fix: Help > Troubleshooting > Enable Developer Mode, then re-run.")
		return nil
	case mcpconfig.StateConfigEmpty:
		fmt.Printf("✓ Config exists at %s\n", path)
		fmt.Println("✗ No MCP servers configured yet.")
		fmt.Println("  Fix: run `td mcp install` to add the tokendog server.")
		return nil
	case mcpconfig.StateOtherServers:
		fmt.Printf("✓ Config exists at %s\n", path)
		fmt.Printf("✓ %d other MCP server(s) configured\n", otherServerCount(cfg))
		fmt.Println("✗ tokendog is not in the server list.")
		fmt.Println("  Fix: run `td mcp install` (won't touch your existing servers).")
		return nil
	case mcpconfig.StateInstalled:
		fmt.Printf("✓ Config exists at %s\n", path)
		fmt.Println("✓ tokendog is installed in mcpServers")
		// Validate the configured binary path actually points at something runnable.
		entry := readInstalledEntry(cfg)
		if entry.Command == "" {
			fmt.Println("✗ tokendog entry has no command field. Fix: run `td mcp install` to refresh.")
			return nil
		}
		if _, err := os.Stat(entry.Command); err != nil {
			fmt.Printf("✗ Configured binary %q is missing or unreadable: %v\n", entry.Command, err)
			fmt.Println("  Fix: run `td mcp install` so the path matches your current install.")
			return nil
		}
		fmt.Printf("✓ Binary %s exists\n", entry.Command)
		// Spawn `td mcp` and send a single initialize request to make sure
		// it actually responds. Catches PATH-environment issues where
		// Claude Desktop's launchd session can't find the binary.
		if err := pingMCPServer(entry); err != nil {
			fmt.Printf("✗ Server didn't respond to initialize: %v\n", err)
			fmt.Println("  Fix: check that the binary path is in Claude Desktop's launcher PATH.")
			return nil
		}
		fmt.Println("✓ Server responds to initialize")
		fmt.Println("\nAll checks passed. Restart Claude Desktop if you haven't recently.")
		return nil
	}
	return nil
}

// buildEntry constructs the ServerEntry pointing at the currently-running
// td binary. We use the absolute path (via os.Executable) rather than
// "td" because Claude Desktop's launchd session often has a trimmed PATH
// that doesn't include /opt/homebrew/bin or /usr/local/bin.
func buildEntry() (mcpconfig.ServerEntry, error) {
	exe, err := os.Executable()
	if err != nil {
		// Fallback to the bare name; doctor will catch the resulting
		// PATH issue if it bites.
		exe = "td"
	}
	return mcpconfig.ServerEntry{
		Command: exe,
		Args:    []string{"mcp"},
	}, nil
}

func otherServerCount(cfg map[string]any) int {
	servers, _ := cfg["mcpServers"].(map[string]any)
	count := 0
	for k := range servers {
		if k != mcpconfig.ServerName {
			count++
		}
	}
	return count
}

func readInstalledEntry(cfg map[string]any) mcpconfig.ServerEntry {
	servers, _ := cfg["mcpServers"].(map[string]any)
	td, _ := servers[mcpconfig.ServerName].(map[string]any)
	cmd, _ := td["command"].(string)
	var args []string
	if a, ok := td["args"].([]any); ok {
		for _, x := range a {
			if s, ok := x.(string); ok {
				args = append(args, s)
			}
		}
	}
	return mcpconfig.ServerEntry{Command: cmd, Args: args}
}

// pingMCPServer spawns the configured td binary and sends a single
// initialize request. Returns nil on a valid-looking JSON-RPC response.
// Times out after a few seconds — the server should respond instantly.
func pingMCPServer(entry mcpconfig.ServerEntry) error {
	cmd := exec.Command(entry.Command, entry.Args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	req := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n"
	if _, err := stdin.Write([]byte(req)); err != nil {
		return err
	}

	dec := json.NewDecoder(stdout)
	var resp map[string]any
	if err := dec.Decode(&resp); err != nil {
		return err
	}
	if resp["jsonrpc"] != "2.0" {
		return fmt.Errorf("unexpected response: %v", resp)
	}
	return nil
}
