package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"tokendog/internal/welcome"
)

var Version = "dev"

var rootCmd = &cobra.Command{
	Use:     "td",
	Short:   "TokenDog — token-optimized CLI proxy for AI coding assistants",
	Version: Version,
	Run: func(cmd *cobra.Command, args []string) {
		// First-run UX: when invoked with no args and the marker is missing,
		// show the welcome screen instead of plain help.
		if welcome.IsFirstRun() {
			welcome.Render(Version)
			_ = welcome.MarkInitialized()
			return
		}
		_ = cmd.Help()
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(welcomeCmd)
	rootCmd.AddCommand(hookCmd)
	rootCmd.AddCommand(gitCmd)
	rootCmd.AddCommand(lsCmd)
	rootCmd.AddCommand(findCmd)
	rootCmd.AddCommand(dockerCmd)
	rootCmd.AddCommand(jqCmd)
	rootCmd.AddCommand(curlCmd)
	rootCmd.AddCommand(kubectlCmd)
	rootCmd.AddCommand(gainCmd)
	rootCmd.AddCommand(rewriteCmd)
	rootCmd.AddCommand(discoverCmd)
	rootCmd.AddCommand(ghCmd)
	rootCmd.AddCommand(pytestCmd)
	rootCmd.AddCommand(jestCmd)
	rootCmd.AddCommand(vitestCmd)
	rootCmd.AddCommand(goCmd)
	rootCmd.AddCommand(cargoCmd)
	rootCmd.AddCommand(npmCmd)
	rootCmd.AddCommand(pnpmCmd)
	rootCmd.AddCommand(yarnCmd)
	rootCmd.AddCommand(pipCmd)
	rootCmd.AddCommand(awsCmd)
	rootCmd.AddCommand(gcloudCmd)
	rootCmd.AddCommand(azCmd)
	rootCmd.AddCommand(makeCmd)
}
