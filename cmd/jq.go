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

var jqCmd = &cobra.Command{
	Use:                "jq",
	Short:              "jq with compact JSON output",
	DisableFlagParsing: true,
	RunE:               runJQ,
}

func runJQ(_ *cobra.Command, args []string) error {
	start := time.Now()
	c := exec.Command("jq", args...)
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	out, err := c.Output()
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		fmt.Print(string(out))
		return err
	}

	raw := string(out)
	filtered := filter.JQ(raw)
	fmt.Print(filtered)

	_ = analytics.Save(analytics.Record{
		Command:       "td jq " + strings.Join(args, " "),
		Timestamp:     time.Now(),
		RawBytes:      len(raw),
		FilteredBytes: len(filtered),
		DurationMs:    elapsed,
	})
	return nil
}
