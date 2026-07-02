package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/spf13/cobra"
	"tokendog/internal/sessioncost"
	"tokendog/internal/statusline"
)

var statuslineWrap string

var statuslineCmd = &cobra.Command{
	Use:   "statusline",
	Short: "Render TokenDog's status line and capture Claude Code's session cost",
	Long: `Renders TokenDog's own status line from the JSON Claude Code pipes on stdin
(directory, git branch, model, context usage, cost) and records the session's
cost.total_cost_usd — the same figure /cost and /usage show — into
~/.config/tokendog/session-costs.jsonl, so the menu bar and td spend can report
Claude Code's own numbers.

Register it in ~/.claude/settings.json (td setup does this for you):

  "statusLine": { "type": "command", "command": "td statusline" }

--wrap <cmd> runs your own status line command with the same stdin instead of
rendering TokenDog's, while still capturing the cost.`,
	RunE:          runStatusline,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	statuslineCmd.Flags().StringVar(&statuslineWrap, "wrap", "",
		"Run your own status line command with the same stdin instead of rendering TokenDog's")
}

// statuslineInput is the subset of Claude Code's statusLine stdin payload we
// use. Unlisted fields are ignored by encoding/json.
type statuslineInput struct {
	Cwd       string `json:"cwd"`
	SessionID string `json:"session_id"`
	Model     struct {
		DisplayName string `json:"display_name"`
	} `json:"model"`
	Workspace struct {
		CurrentDir string `json:"current_dir"`
	} `json:"workspace"`
	Cost struct {
		TotalCostUSD float64 `json:"total_cost_usd"`
	} `json:"cost"`
	ContextWindow struct {
		UsedPercentage float64 `json:"used_percentage"`
	} `json:"context_window"`
	Exceeds200k bool `json:"exceeds_200k_tokens"`
	Effort      struct {
		Level string `json:"level"`
	} `json:"effort"`
}

func runStatusline(_ *cobra.Command, _ []string) error {
	// Read the whole payload up front: we parse it and, when wrapping, replay it
	// verbatim to the wrapped command's stdin.
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		raw = nil
	}
	in := parseStatusline(raw)

	// Capture is best-effort and must never break the render: a parse or write
	// failure is swallowed.
	if in != nil && in.SessionID != "" {
		_ = sessioncost.Append(sessioncost.Sample{
			SessionID: in.SessionID,
			Model:     in.Model.DisplayName,
			CostUSD:   in.Cost.TotalCostUSD,
			UpdatedAt: time.Now(),
		})
	}

	if statuslineWrap != "" {
		return runWrapped(statuslineWrap, raw)
	}
	if in != nil {
		dir := in.Workspace.CurrentDir
		if dir == "" {
			dir = in.Cwd
		}
		fmt.Println(statusline.Render(statusline.Payload{
			Dir:         dir,
			Model:       in.Model.DisplayName,
			Effort:      in.Effort.Level,
			ContextPct:  in.ContextWindow.UsedPercentage,
			Exceeds200k: in.Exceeds200k,
			CostUSD:     in.Cost.TotalCostUSD,
		}))
	}
	return nil
}

func parseStatusline(raw []byte) *statuslineInput {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var in statuslineInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil
	}
	return &in
}

// runWrapped executes a user-provided status line command via the shell
// (matching how Claude Code invokes statusLine commands), feeding it the same
// stdin and forwarding its stdout/stderr. The child's exit code is propagated.
func runWrapped(command string, stdin []byte) error {
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/sh"
	}
	c := exec.Command(sh, "-c", command)
	c.Stdin = bytes.NewReader(stdin)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	err := c.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.ExitCode())
	}
	return err
}
