package cmd

import (
	"github.com/spf13/cobra"
	"tokendog/internal/filter"
)

var pytestCmd = &cobra.Command{
	Use:                "pytest",
	Short:              "pytest with passing-test summary collapse",
	DisableFlagParsing: true,
	RunE:               runPytest,
}

func runPytest(_ *cobra.Command, args []string) error {
	return runTestCommand("pytest", "pytest", args)
}

var jestCmd = &cobra.Command{
	Use:                "jest",
	Short:              "jest with passing-test summary collapse",
	DisableFlagParsing: true,
	RunE:               runJest,
}

func runJest(_ *cobra.Command, args []string) error {
	return runTestCommand("jest", "jest", args)
}

var vitestCmd = &cobra.Command{
	Use:                "vitest",
	Short:              "vitest with passing-test summary collapse",
	DisableFlagParsing: true,
	RunE:               runVitest,
}

func runVitest(_ *cobra.Command, args []string) error {
	return runTestCommand("vitest", "vitest", args)
}

func runTestCommand(binary, runner string, args []string) error {
	return runFiltered(binary, args, func(raw string) string {
		return filter.Test(runner, raw)
	}, "td "+binary+" ")
}
