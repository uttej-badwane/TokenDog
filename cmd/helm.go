package cmd

import "github.com/spf13/cobra"

var helmCmd = &cobra.Command{
	Use:                "helm",
	Short:              "helm with table normalization and blank-line collapse",
	DisableFlagParsing: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return runFiltered("helm", args, "td helm ")
	},
}
