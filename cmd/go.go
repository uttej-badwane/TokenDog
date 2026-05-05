package cmd

import (
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"tokendog/internal/filter"
)

var goCmd = &cobra.Command{
	Use:                "go",
	Short:              "go (only `go test` output is filtered; other subcommands pass through)",
	DisableFlagParsing: true,
	RunE:               runGo,
}

func runGo(_ *cobra.Command, args []string) error {
	subcmd := ""
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			subcmd = arg
			break
		}
	}

	// Non-test subcommands pass through unchanged. We don't want to compress
	// `go build` output (compiler errors are user content) or `go run`
	// (program output the model is investigating).
	if subcmd != "test" {
		c := exec.Command("go", args...)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		c.Stdin = os.Stdin
		return c.Run()
	}

	return runFiltered("go", args, func(raw string) string {
		return filter.Test("go", raw)
	}, "td go ")
}
