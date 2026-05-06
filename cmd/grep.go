package cmd

import "github.com/spf13/cobra"

var grepCmd = &cobra.Command{
	Use:                "grep",
	Short:              "grep with matches grouped by file path",
	DisableFlagParsing: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return runFiltered("grep", args, "td grep ")
	},
}
