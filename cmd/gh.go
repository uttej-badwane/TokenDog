package cmd

import (
	"github.com/spf13/cobra"
	"tokendog/internal/filter"
)

var ghCmd = &cobra.Command{
	Use:                "gh",
	Short:              "gh (GitHub CLI) with compact table output",
	DisableFlagParsing: true,
	RunE:               runGH,
}

func runGH(_ *cobra.Command, args []string) error {
	subcmd := extractSubcommand(args, ghValueFlags)
	return runFiltered("gh", args, func(raw string) string {
		return filter.GH(subcmd, raw)
	}, "td gh ")
}
