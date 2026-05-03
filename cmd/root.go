package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var Version = "dev"

var rootCmd = &cobra.Command{
	Use:     "td",
	Short:   "TokenDog — token-optimized CLI proxy for AI coding assistants",
	Version: Version,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(hookCmd)
	rootCmd.AddCommand(gitCmd)
	rootCmd.AddCommand(lsCmd)
	rootCmd.AddCommand(findCmd)
	rootCmd.AddCommand(dockerCmd)
	rootCmd.AddCommand(gainCmd)
	rootCmd.AddCommand(rewriteCmd)
}
