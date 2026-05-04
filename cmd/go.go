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

var goCmd = &cobra.Command{
	Use:                "go",
	Short:              "go (only `go test` output is filtered; other subcommands pass through)",
	DisableFlagParsing: true,
	RunE:               runGo,
}

func runGo(_ *cobra.Command, args []string) error {
	subcmd := ""
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			subcmd = arg
			break
		}
	}

	// Non-test subcommands pass through unchanged. We don't want to compress
	// `go build` output (compiler errors are user content) or `go run`
	// (program output the model is investigating).
	if subcmd != "test" {
		c := exec.Command("go", args...)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		c.Stdin = os.Stdin
		return c.Run()
	}

	start := time.Now()
	c := exec.Command("go", args...)
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	out, _ := c.Output()
	elapsed := time.Since(start).Milliseconds()

	raw := string(out)
	filtered := filter.Test("go", raw)
	fmt.Print(filtered)

	_ = analytics.Save(analytics.Record{
		Command:       "td go " + strings.Join(args, " "),
		Timestamp:     time.Now(),
		RawBytes:      len(raw),
		FilteredBytes: len(filtered),
		DurationMs:    elapsed,
	})
	return nil
}
