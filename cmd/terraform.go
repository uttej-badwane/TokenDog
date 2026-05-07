package cmd

import "github.com/spf13/cobra"

// terraformCmd / tofuCmd share the same filter (OpenTofu's plan/apply
// output is the same shape as terraform's). Two separate cobra entries
// because they're two separate binaries the hook rewrites.
func tfCmd(name string) *cobra.Command {
	return &cobra.Command{
		Use:                name,
		Short:              name + " with refresh/apply-progress noise stripped (resource diffs preserved)",
		DisableFlagParsing: true,
		RunE: func(_ *cobra.Command, args []string) error {
			return runFiltered(name, args, "td "+name+" ")
		},
	}
}

var (
	terraformCmd = tfCmd("terraform")
	tofuCmd      = tfCmd("tofu")
)
