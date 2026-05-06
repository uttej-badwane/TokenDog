package cmd

import "github.com/spf13/cobra"

var jqCmd = &cobra.Command{
	Use:                "jq",
	Short:              "jq with compact JSON output",
	DisableFlagParsing: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return runFiltered("jq", args, "td jq ")
	},
}
