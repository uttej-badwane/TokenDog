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

var gitCmd = &cobra.Command{
	Use:                "git",
	Short:              "Git commands with compact output",
	DisableFlagParsing: true,
	RunE:               runGit,
}

func runGit(_ *cobra.Command, args []string) error {
	subcmd := ""
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			subcmd = arg
			break
		}
	}

	start := time.Now()
	c := exec.Command("git", args...)
	c.Stderr = os.Stderr
	out, err := c.Output()
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		if len(out) > 0 {
			fmt.Print(string(out))
		}
		return err
	}

	raw := string(out)
	filtered := filter.Git(subcmd, raw)
	fmt.Print(filtered)

	_ = analytics.Save(analytics.Record{
		Command:       "td git " + strings.Join(args, " "),
		Timestamp:     time.Now(),
		RawBytes:      len(raw),
		FilteredBytes: len(filtered),
		DurationMs:    elapsed,
	})
	return nil
}
