package cmd

import (
	"strings"

	"github.com/spf13/cobra"
	"tokendog/internal/filter"
)

var lsCmd = &cobra.Command{
	Use:                "ls",
	Short:              "List files with compact output",
	DisableFlagParsing: true,
	RunE:               runLs,
}

func runLs(_ *cobra.Command, args []string) error {
	// Ensure -l flag so output is parseable. We mutate the args we pass to
	// exec while keeping the original for the analytics label.
	lsArgs := args
	hasLong := false
	for _, a := range args {
		if strings.HasPrefix(a, "-") && strings.ContainsRune(a, 'l') {
			hasLong = true
			break
		}
	}
	if !hasLong {
		lsArgs = append([]string{"-la"}, args...)
	}
	return runFiltered("ls", lsArgs, filter.Ls, "td ls ")
}
