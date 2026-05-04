package cmd

import (
	"github.com/spf13/cobra"
	"tokendog/internal/filter"
)

var npmCmd = &cobra.Command{
	Use:                "npm",
	Short:              "npm with progress noise stripped",
	DisableFlagParsing: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return runFiltered("npm", args, filter.PackageManager, "td npm ")
	},
}

var pnpmCmd = &cobra.Command{
	Use:                "pnpm",
	Short:              "pnpm with progress noise stripped",
	DisableFlagParsing: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return runFiltered("pnpm", args, filter.PackageManager, "td pnpm ")
	},
}

var yarnCmd = &cobra.Command{
	Use:                "yarn",
	Short:              "yarn with progress noise stripped",
	DisableFlagParsing: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return runFiltered("yarn", args, filter.PackageManager, "td yarn ")
	},
}

var pipCmd = &cobra.Command{
	Use:                "pip",
	Short:              "pip with progress noise stripped",
	DisableFlagParsing: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return runFiltered("pip", args, filter.PackageManager, "td pip ")
	},
}
