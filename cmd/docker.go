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

var dockerCmd = &cobra.Command{
	Use:                "docker",
	Short:              "Docker commands with compact output",
	DisableFlagParsing: true,
	RunE:               runDocker,
}

func runDocker(_ *cobra.Command, args []string) error {
	subcmd := ""
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			subcmd = arg
			break
		}
	}

	start := time.Now()
	c := exec.Command("docker", args...)
	c.Stderr = os.Stderr
	out, err := c.Output()
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		fmt.Print(string(out))
		return err
	}

	raw := string(out)
	filtered := filter.Docker(subcmd, raw)
	fmt.Print(filtered)

	_ = analytics.Save(analytics.Record{
		Command:       "td docker " + strings.Join(args, " "),
		Timestamp:     time.Now(),
		RawBytes:      len(raw),
		FilteredBytes: len(filtered),
		DurationMs:    elapsed,
	})
	return nil
}
