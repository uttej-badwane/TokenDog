package cmd

import "github.com/spf13/cobra"

var curlCmd = &cobra.Command{
	Use:                "curl",
	Short:              "curl with JSON-aware response compression",
	DisableFlagParsing: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return runFiltered("curl", args, "td curl ")
	},
}
