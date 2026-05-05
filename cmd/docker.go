package cmd

import (
	"github.com/spf13/cobra"
	"tokendog/internal/filter"
)

var dockerCmd = &cobra.Command{
	Use:                "docker",
	Short:              "Docker commands with compact output",
	DisableFlagParsing: true,
	RunE:               runDocker,
}

func runDocker(_ *cobra.Command, args []string) error {
	subcmd := extractSubcommand(args, dockerValueFlags)
	return runFiltered("docker", args, func(raw string) string {
		return filter.Docker(subcmd, raw)
	}, "td docker ")
}
