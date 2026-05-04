package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"tokendog/internal/analytics"
	"tokendog/internal/filter"
)

var cargoCmd = &cobra.Command{
	Use:                "cargo",
	Short:              "cargo (test/build/check filtered; other subcommands pass through)",
	DisableFlagParsing: true,
	RunE:               runCargo,
}

func runCargo(_ *cobra.Command, args []string) error {
	subcmd := ""
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			subcmd = arg
			break
		}
	}

	switch subcmd {
	case "test":
		return runFiltered("cargo", args, func(raw string) string {
			return filter.Test("cargo", raw)
		}, "td cargo ")
	case "build", "check", "fetch", "update":
		return runFiltered("cargo", args, filter.PackageManager, "td cargo ")
	default:
		c := exec.Command("cargo", args...)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		c.Stdin = os.Stdin
		return c.Run()
	}
}

// runFiltered is a generic exec helper used by package-manager / test-style
// wrappers — captures stdout, applies a filter, prints filtered output, and
// records analytics. Stderr streams through live so progress is visible.
func runFiltered(binary string, args []string, fn func(string) string, recordPrefix string) error {
	start := time.Now()
	c := exec.Command(binary, args...)
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	out, err := c.Output()
	elapsed := time.Since(start).Milliseconds()

	raw := string(out)
	filtered := fn(raw)
	fmt.Print(filtered)

	_ = analytics.Save(analytics.Record{
		Command:       recordPrefix + strings.Join(args, " "),
		Timestamp:     time.Now(),
		RawBytes:      len(raw),
		FilteredBytes: len(filtered),
		DurationMs:    elapsed,
	})
	return err
}
