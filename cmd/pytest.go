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

var pytestCmd = &cobra.Command{
	Use:                "pytest",
	Short:              "pytest with passing-test summary collapse",
	DisableFlagParsing: true,
	RunE:               runPytest,
}

func runPytest(_ *cobra.Command, args []string) error {
	return runTestCommand("pytest", "pytest", args)
}

var jestCmd = &cobra.Command{
	Use:                "jest",
	Short:              "jest with passing-test summary collapse",
	DisableFlagParsing: true,
	RunE:               runJest,
}

func runJest(_ *cobra.Command, args []string) error {
	return runTestCommand("jest", "jest", args)
}

var vitestCmd = &cobra.Command{
	Use:                "vitest",
	Short:              "vitest with passing-test summary collapse",
	DisableFlagParsing: true,
	RunE:               runVitest,
}

func runVitest(_ *cobra.Command, args []string) error {
	return runTestCommand("vitest", "vitest", args)
}

func runTestCommand(binary, runner string, args []string) error {
	start := time.Now()
	c := exec.Command(binary, args...)
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	out, err := c.Output()
	elapsed := time.Since(start).Milliseconds()

	raw := string(out)
	// On error we still want to filter — the filter detects failure signals
	// and falls back to verbatim output. This keeps savings on flaky-but-
	// passing cases while never hiding real failures.
	filtered := filter.Test(runner, raw)
	fmt.Print(filtered)

	_ = analytics.Save(analytics.Record{
		Command:       "td " + binary + " " + strings.Join(args, " "),
		Timestamp:     time.Now(),
		RawBytes:      len(raw),
		FilteredBytes: len(filtered),
		DurationMs:    elapsed,
	})
	return err
}
