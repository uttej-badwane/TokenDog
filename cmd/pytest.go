package cmd

import "github.com/spf13/cobra"

// testCmd builds a cobra wrapper for pytest/jest/vitest — same Test filter
// (passing-summary collapse, verbatim on failure). Only the binary name
// differs.
func testCmd(name string) *cobra.Command {
	return &cobra.Command{
		Use:                name,
		Short:              name + " with passing-test summary collapse",
		DisableFlagParsing: true,
		RunE: func(_ *cobra.Command, args []string) error {
			return runFiltered(name, args, "td "+name+" ")
		},
	}
}

var (
	pytestCmd = testCmd("pytest")
	jestCmd   = testCmd("jest")
	vitestCmd = testCmd("vitest")
)
