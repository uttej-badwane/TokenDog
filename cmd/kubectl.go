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

var kubectlCmd = &cobra.Command{
	Use:                "kubectl",
	Short:              "kubectl with compact output (get/describe/top)",
	DisableFlagParsing: true,
	RunE:               runKubectl,
}

func runKubectl(_ *cobra.Command, args []string) error {
	subcmd := extractSubcommand(args, kubectlValueFlags)

	start := time.Now()
	c := exec.Command("kubectl", args...)
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	out, err := c.Output()
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		fmt.Print(string(out))
		return err
	}

	raw := string(out)
	filtered := filter.Kubectl(subcmd, raw)
	fmt.Print(filtered)

	_ = analytics.Save(analytics.Record{
		Command:       "td kubectl " + strings.Join(args, " "),
		Timestamp:     time.Now(),
		RawBytes:      len(raw),
		FilteredBytes: len(filtered),
		DurationMs:    elapsed,
	})
	return nil
}
