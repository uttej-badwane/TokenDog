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

var findCmd = &cobra.Command{
	Use:                "find",
	Short:              "Find files with compact grouped output",
	DisableFlagParsing: true,
	RunE:               runFind,
}

func runFind(_ *cobra.Command, args []string) error {
	start := time.Now()
	c := exec.Command("find", args...)
	c.Stderr = os.Stderr
	out, err := c.Output()
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		fmt.Print(string(out))
		return err
	}

	raw := string(out)
	filtered := filter.Find(raw)
	fmt.Print(filtered)

	_ = analytics.Save(analytics.Record{
		Command:       "td find " + strings.Join(args, " "),
		Timestamp:     time.Now(),
		RawBytes:      len(raw),
		FilteredBytes: len(filtered),
		DurationMs:    elapsed,
	})
	return nil
}
