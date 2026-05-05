package cmd

import (
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"tokendog/internal/filter"
)

var cargoCmd = &cobra.Command{
	Use:                "cargo",
	Short:              "cargo (test/build/check filtered; other subcommands pass through)",
	DisableFlagParsing: true,
	RunE:               runCargo,
}

func runCargo(_ *cobra.Command, args []string) error {
	subcmd := extractSubcommand(args, cargoValueFlags)

	switch subcmd {
	case "test":
		return runFiltered("cargo", args, func(raw string) string {
			return filter.Test("cargo", raw)
		}, "td cargo ")
	case "build", "check", "fetch", "update":
		return runFiltered("cargo", args, filter.PackageManager, "td cargo ")
	default:
		c := exec.Command("cargo", args...)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		c.Stdin = os.Stdin
		return c.Run()
	}
}
