package cmd

import (
	"github.com/spf13/cobra"
	"tokendog/internal/filter"
)

var awsCmd = &cobra.Command{
	Use:                "aws",
	Short:              "aws CLI with JSON/table compaction (lossless)",
	DisableFlagParsing: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return runFiltered("aws", args, filter.Cloud, "td aws ")
	},
}

var gcloudCmd = &cobra.Command{
	Use:                "gcloud",
	Short:              "gcloud CLI with JSON/YAML/table compaction (lossless)",
	DisableFlagParsing: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return runFiltered("gcloud", args, filter.Cloud, "td gcloud ")
	},
}

var azCmd = &cobra.Command{
	Use:                "az",
	Short:              "az (Azure) CLI with JSON/table compaction (lossless)",
	DisableFlagParsing: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return runFiltered("az", args, filter.Cloud, "td az ")
	},
}
