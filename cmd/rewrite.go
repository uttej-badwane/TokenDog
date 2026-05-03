package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"tokendog/internal/hook"
)

var rewriteCmd = &cobra.Command{
	Use:   "rewrite [command...]",
	Short: "Show how td would rewrite a command (debug)",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runRewrite,
}

func runRewrite(_ *cobra.Command, args []string) error {
	cmd := strings.Join(args, " ")
	rewritten := hook.RewriteCommand(cmd)
	if rewritten == cmd {
		fmt.Printf("passthrough (no rewrite): %s\n", cmd)
	} else {
		fmt.Printf("%s\n  → %s\n", cmd, rewritten)
	}
	return nil
}
