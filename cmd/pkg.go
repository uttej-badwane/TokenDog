package cmd

import "github.com/spf13/cobra"

// pkgCmd builds a cobra wrapper for npm/pnpm/yarn/pip — they all share the
// PackageManager filter so the cobra wiring is identical aside from the
// binary name.
func pkgCmd(name string) *cobra.Command {
	return &cobra.Command{
		Use:                name,
		Short:              name + " with progress noise stripped",
		DisableFlagParsing: true,
		RunE: func(_ *cobra.Command, args []string) error {
			return runFiltered(name, args, "td "+name+" ")
		},
	}
}

var (
	npmCmd  = pkgCmd("npm")
	pnpmCmd = pkgCmd("pnpm")
	yarnCmd = pkgCmd("yarn")
	pipCmd  = pkgCmd("pip")
)
