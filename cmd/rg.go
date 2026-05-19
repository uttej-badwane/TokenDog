package cmd

import "github.com/spf13/cobra"

var rgCmd = &cobra.Command{
	Use:                "rg",
	Short:              "rg (ripgrep) with matches grouped by file path",
	DisableFlagParsing: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return runFiltered("rg", args, "td rg ")
	},
}
