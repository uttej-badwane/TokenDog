package cmd

import "github.com/spf13/cobra"

// cloudCmd builds a cobra wrapper for aws/gcloud/az — same cloud filter
// (lossless JSON/YAML/table compaction). Only the binary name + short
// help text differ.
func cloudCmd(name, short string) *cobra.Command {
	return &cobra.Command{
		Use:                name,
		Short:              short,
		DisableFlagParsing: true,
		RunE: func(_ *cobra.Command, args []string) error {
			return runFiltered(name, args, "td "+name+" ")
		},
	}
}

var (
	awsCmd    = cloudCmd("aws", "aws CLI with JSON/table compaction (lossless)")
	gcloudCmd = cloudCmd("gcloud", "gcloud CLI with JSON/YAML/table compaction (lossless)")
	azCmd     = cloudCmd("az", "az (Azure) CLI with JSON/table compaction (lossless)")
)
