package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"tokendog/internal/welcome"
)

var Version = "dev"

var rootCmd = &cobra.Command{
	Use:     "td",
	Short:   "TokenDog — token-optimized CLI proxy for AI coding assistants",
	Version: Version,
	// All wrapper subcommands forward the wrapped tool's stderr verbatim.
	// When that tool exits non-zero (e.g. `git status` in a non-repo) we
	// already have the relevant error visible to the user — Cobra's default
	// "Error: ..." line and usage banner just adds noise on top.
	SilenceUsage:  true,
	SilenceErrors: true,
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
	err := rootCmd.Execute()
	if err == nil {
		return
	}
	// When a wrapper subcommand failed, propagate the wrapped tool's exit
	// code without re-printing the error — its stderr already reached the
	// user. Other errors (Cobra parsing, our own logic) get a brief message.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.ExitCode())
	}
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
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
	rootCmd.AddCommand(replayCmd)
	rootCmd.AddCommand(filterCmd)
	rootCmd.AddCommand(mcpCmd)
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
	rootCmd.AddCommand(grepCmd)
	rootCmd.AddCommand(terraformCmd)
	rootCmd.AddCommand(tofuCmd)
	rootCmd.AddCommand(purgeCmd)
	rootCmd.AddCommand(proxyCmd)
	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(unsetupCmd)
}
