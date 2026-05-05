package cmd

import (
	"github.com/spf13/cobra"
	"tokendog/internal/filter"
)

var curlCmd = &cobra.Command{
	Use:                "curl",
	Short:              "curl with JSON-aware response compression",
	DisableFlagParsing: true,
	RunE:               runCurl,
}

func runCurl(_ *cobra.Command, args []string) error {
	return runFiltered("curl", args, filter.Curl, "td curl ")
}
