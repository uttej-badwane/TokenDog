package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"tokendog/internal/hook"
)

var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Hook processors for AI coding assistants",
}

var hookClaudeCmd = &cobra.Command{
	Use:   "claude",
	Short: "Process Claude Code PreToolUse hooks",
	RunE:  runHookClaude,
}

func init() {
	hookCmd.AddCommand(hookClaudeCmd)
}

func runHookClaude(_ *cobra.Command, _ []string) error {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}

	var input hook.ClaudeHookInput
	if err := json.Unmarshal(data, &input); err != nil {
		return nil
	}

	output := hook.ProcessClaude(input)
	if output == nil {
		return nil
	}

	out, err := json.Marshal(output)
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}
