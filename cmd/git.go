package cmd

import "github.com/spf13/cobra"

var gitCmd = &cobra.Command{
	Use:                "git",
	Short:              "Git commands with compact output",
	DisableFlagParsing: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return runFiltered("git", args, "td git ")
	},
}
