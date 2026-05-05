package cmd

import (
	"github.com/spf13/cobra"
	"tokendog/internal/filter"
)

var gitCmd = &cobra.Command{
	Use:                "git",
	Short:              "Git commands with compact output",
	DisableFlagParsing: true,
	RunE:               runGit,
}

func runGit(_ *cobra.Command, args []string) error {
	subcmd := extractSubcommand(args, gitValueFlags)
	return runFiltered("git", args, func(raw string) string {
		return filter.Git(subcmd, raw)
	}, "td git ")
}
