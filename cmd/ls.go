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

var lsCmd = &cobra.Command{
	Use:                "ls",
	Short:              "List files with compact output",
	DisableFlagParsing: true,
	RunE:               runLs,
}

func runLs(_ *cobra.Command, args []string) error {
	// Ensure -l flag so output is parseable
	lsArgs := args
	hasLong := false
	for _, a := range args {
		if strings.HasPrefix(a, "-") && strings.ContainsRune(a, 'l') {
			hasLong = true
			break
		}
	}
	if !hasLong {
		lsArgs = append([]string{"-la"}, args...)
	}

	start := time.Now()
	c := exec.Command("ls", lsArgs...)
	c.Stderr = os.Stderr
	out, err := c.Output()
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		fmt.Print(string(out))
		return err
	}

	raw := string(out)
	filtered := filter.Ls(raw)
	fmt.Print(filtered)

	_ = analytics.Save(analytics.Record{
		Command:       "td ls " + strings.Join(args, " "),
		Timestamp:     time.Now(),
		RawBytes:      len(raw),
		FilteredBytes: len(filtered),
		DurationMs:    elapsed,
	})
	return nil
}
