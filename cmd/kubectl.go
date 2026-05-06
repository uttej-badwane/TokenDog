package cmd

import "github.com/spf13/cobra"

var kubectlCmd = &cobra.Command{
	Use:                "kubectl",
	Short:              "kubectl with compact output (get/describe/top)",
	DisableFlagParsing: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return runFiltered("kubectl", args, "td kubectl ")
	},
}
