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

var ghCmd = &cobra.Command{
	Use:                "gh",
	Short:              "gh (GitHub CLI) with compact table output",
	DisableFlagParsing: true,
	RunE:               runGH,
}

func runGH(_ *cobra.Command, args []string) error {
	subcmd := extractSubcommand(args, ghValueFlags)

	start := time.Now()
	c := exec.Command("gh", args...)
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	out, err := c.Output()
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		fmt.Print(string(out))
		return err
	}

	raw := string(out)
	filtered := filter.GH(subcmd, raw)
	fmt.Print(filtered)

	_ = analytics.Save(analytics.Record{
		Command:       "td gh " + strings.Join(args, " "),
		Timestamp:     time.Now(),
		RawBytes:      len(raw),
		FilteredBytes: len(filtered),
		DurationMs:    elapsed,
	})
	return nil
}
