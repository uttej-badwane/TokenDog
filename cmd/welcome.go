package cmd

import (
	"github.com/spf13/cobra"
	"tokendog/internal/welcome"
)

var welcomeCmd = &cobra.Command{
	Use:   "welcome",
	Short: "Show the welcome screen with setup status",
	RunE: func(_ *cobra.Command, _ []string) error {
		welcome.Render(Version)
		return welcome.MarkInitialized()
	},
}
