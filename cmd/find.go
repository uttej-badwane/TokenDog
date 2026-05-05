package cmd

import (
	"github.com/spf13/cobra"
	"tokendog/internal/filter"
)

var findCmd = &cobra.Command{
	Use:                "find",
	Short:              "Find files with compact grouped output",
	DisableFlagParsing: true,
	RunE:               runFind,
}

func runFind(_ *cobra.Command, args []string) error {
	return runFiltered("find", args, filter.Find, "td find ")
}
