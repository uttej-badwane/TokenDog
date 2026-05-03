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

var curlCmd = &cobra.Command{
	Use:                "curl",
	Short:              "curl with JSON-aware response compression",
	DisableFlagParsing: true,
	RunE:               runCurl,
}

func runCurl(_ *cobra.Command, args []string) error {
	start := time.Now()
	c := exec.Command("curl", args...)
	c.Stderr = os.Stderr
	out, err := c.Output()
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		fmt.Print(string(out))
		return err
	}

	raw := string(out)
	filtered := filter.Curl(raw)
	fmt.Print(filtered)

	_ = analytics.Save(analytics.Record{
		Command:       "td curl " + strings.Join(args, " "),
		Timestamp:     time.Now(),
		RawBytes:      len(raw),
		FilteredBytes: len(filtered),
		DurationMs:    elapsed,
	})
	return nil
}
