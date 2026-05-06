package cmd

import (
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"tokendog/internal/filter"
)

var cargoCmd = &cobra.Command{
	Use:                "cargo",
	Short:              "cargo (test/build/check filtered; other subcommands pass through)",
	DisableFlagParsing: true,
	RunE:               runCargo,
}

// runCargo defers to the registry for filtering, but for unsupported
// subcommands (run, fmt, doc, etc.) we exec without capturing — the user
// wants the live output verbatim.
func runCargo(_ *cobra.Command, args []string) error {
	if _, applied := filter.Apply("cargo", args, ""); !applied {
		// Filter declined to handle (registry returned applied=false). This
		// shouldn't actually happen since cargo is registered, but defend
		// for symmetry.
		return passthrough("cargo", args)
	}
	// The cargo adapter returns raw unchanged for unsupported subcommands,
	// so calling runFiltered would still capture stdout (which is what we
	// want — the user gets a passthrough but we record analytics noting
	// zero savings, which honestly reflects what happened).
	return runFiltered("cargo", args, "td cargo ")
}

func passthrough(binary string, args []string) error {
	c := exec.Command(binary, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	return c.Run()
}
