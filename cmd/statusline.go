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
)

var statuslineWrap string

var statuslineCmd = &cobra.Command{
	Use:   "statusline",
	Short: "Capture Claude Code's own session cost from the statusLine payload",
	Long: `Reads the JSON Claude Code pipes to a statusLine command on stdin, records
the session's cost.total_cost_usd (the same figure /cost and /usage show) into
~/.config/tokendog/session-costs.jsonl, and — with --wrap — runs your existing
statusline command with the same stdin so its display is unchanged.

Register it in ~/.claude/settings.json (td setup can do this for you):

  "statusLine": {
    "type": "command",
    "command": "td statusline --wrap 'npx -y ccstatusline@latest'"
  }

Without --wrap it prints a minimal "model · $cost" line of its own.`,
	RunE:          runStatusline,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	statuslineCmd.Flags().StringVar(&statuslineWrap, "wrap", "",
		"Shell command for your existing statusline; run with the same stdin so its output is preserved")
}

// statuslineInput is the subset of Claude Code's statusLine stdin payload we
// need. Unlisted fields are ignored by encoding/json.
type statuslineInput struct {
	SessionID string `json:"session_id"`
	Model     struct {
		DisplayName string `json:"display_name"`
	} `json:"model"`
	Cost struct {
		TotalCostUSD float64 `json:"total_cost_usd"`
	} `json:"cost"`
}

func runStatusline(_ *cobra.Command, _ []string) error {
	// Read the whole payload up front: we both parse it and (when wrapping)
	// replay it verbatim to the wrapped command's stdin.
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		raw = nil
	}

	// Capture is best-effort and must never break the statusline render: a
	// parse or write failure is swallowed so the wrapped command still runs.
	if in := parseStatusline(raw); in != nil && in.SessionID != "" {
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
	// No wrapped command: render our own minimal line so the statusline isn't
	// blank when TokenDog is the sole statusLine provider.
	if in := parseStatusline(raw); in != nil {
		fmt.Printf("%s · $%.2f\n", in.Model.DisplayName, in.Cost.TotalCostUSD)
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

// runWrapped executes the user's existing statusline command via the shell
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
