package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"tokendog/internal/analytics"
)

var gainHistory bool

var gainCmd = &cobra.Command{
	Use:   "gain",
	Short: "Show token savings summary",
	RunE:  runGain,
}

func init() {
	gainCmd.Flags().BoolVar(&gainHistory, "history", false, "Show recent command history")
}

func runGain(_ *cobra.Command, _ []string) error {
	records, err := analytics.LoadAll()
	if err != nil {
		return err
	}
	fmt.Print(analytics.RenderGain(records, gainHistory))
	return nil
}
