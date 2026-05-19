package cmd

import "github.com/spf13/cobra"

var psqlCmd = &cobra.Command{
	Use:                "psql",
	Short:              "psql with separator-line removal and column-padding normalization",
	DisableFlagParsing: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return runFiltered("psql", args, "td psql ")
	},
}

var pgcliCmd = &cobra.Command{
	Use:                "pgcli",
	Short:              "pgcli (psql-compatible output compaction)",
	DisableFlagParsing: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return runFiltered("pgcli", args, "td pgcli ")
	},
}
