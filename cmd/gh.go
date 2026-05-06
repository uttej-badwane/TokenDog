package cmd

import "github.com/spf13/cobra"

var ghCmd = &cobra.Command{
	Use:                "gh",
	Short:              "gh (GitHub CLI) with compact table output",
	DisableFlagParsing: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return runFiltered("gh", args, "td gh ")
	},
}
