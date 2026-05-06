package cmd

import "github.com/spf13/cobra"

var findCmd = &cobra.Command{
	Use:                "find",
	Short:              "Find files with compact grouped output",
	DisableFlagParsing: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return runFiltered("find", args, "td find ")
	},
}
