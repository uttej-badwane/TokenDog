package cmd

import "github.com/spf13/cobra"

var dockerCmd = &cobra.Command{
	Use:                "docker",
	Short:              "Docker commands with compact output",
	DisableFlagParsing: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return runFiltered("docker", args, "td docker ")
	},
}
