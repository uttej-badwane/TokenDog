package cmd

import (
	"github.com/spf13/cobra"
	"tokendog/internal/filter"
)

var makeCmd = &cobra.Command{
	Use:                "make",
	Short:              "make with compile spam stripped (warnings/errors preserved)",
	DisableFlagParsing: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return runFiltered("make", args, filter.Make, "td make ")
	},
}
