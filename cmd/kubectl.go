package cmd

import (
	"github.com/spf13/cobra"
	"tokendog/internal/filter"
)

var kubectlCmd = &cobra.Command{
	Use:                "kubectl",
	Short:              "kubectl with compact output (get/describe/top)",
	DisableFlagParsing: true,
	RunE:               runKubectl,
}

func runKubectl(_ *cobra.Command, args []string) error {
	subcmd := extractSubcommand(args, kubectlValueFlags)
	return runFiltered("kubectl", args, func(raw string) string {
		return filter.Kubectl(subcmd, raw)
	}, "td kubectl ")
}
