package cmd

import (
	"github.com/spf13/cobra"
	"tokendog/internal/filter"
)

var jqCmd = &cobra.Command{
	Use:                "jq",
	Short:              "jq with compact JSON output",
	DisableFlagParsing: true,
	RunE:               runJQ,
}

func runJQ(_ *cobra.Command, args []string) error {
	return runFiltered("jq", args, filter.JQ, "td jq ")
}
