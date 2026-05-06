package cmd

import (
	"strings"

	"github.com/spf13/cobra"
)

var goCmd = &cobra.Command{
	Use:                "go",
	Short:              "go (only `go test` output is filtered; other subcommands pass through)",
	DisableFlagParsing: true,
	RunE:               runGo,
}

// runGo decides whether to filter or passthrough based on the subcommand.
// Only `go test` flows through the filter — `go build`, `go run`, `go vet`,
// etc. emit user-relevant content (compiler errors, program output) that
// must reach the model unchanged.
func runGo(_ *cobra.Command, args []string) error {
	if subcmd(args) != "test" {
		return passthrough("go", args)
	}
	return runFiltered("go", args, "td go ")
}

func subcmd(args []string) string {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			return a
		}
	}
	return ""
}
