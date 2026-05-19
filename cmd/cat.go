package cmd

import "github.com/spf13/cobra"

var catCmd = &cobra.Command{
	Use:                "cat",
	Short:              "cat with blank-line collapse and trailing-whitespace strip",
	DisableFlagParsing: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return runFiltered("cat", args, "td cat ")
	},
}

var headCmd = &cobra.Command{
	Use:                "head",
	Short:              "head with blank-line collapse",
	DisableFlagParsing: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return runFiltered("head", args, "td head ")
	},
}

var tailCmd = &cobra.Command{
	Use:                "tail",
	Short:              "tail with blank-line collapse",
	DisableFlagParsing: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return runFiltered("tail", args, "td tail ")
	},
}
