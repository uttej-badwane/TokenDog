package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"tokendog/internal/stash"
)

var stashCmd = &cobra.Command{
	Use:   "stash",
	Short: "Inspect the reversible-compression store of original tool outputs",
	Long: `When reversible compression is on (TD_REVERSIBLE=1), the proxy stashes the
full original of any large tool output and injects a compact preview with a
'[td:STASHED id=… ]' marker. The model recovers the original on demand via the
td_retrieve MCP tool. These subcommands let you inspect that store directly:

  td stash list            # one row per stashed original (newest first)
  td stash get <id>        # print the full original for an id
  td stash purge           # delete every stashed original

Originals live under ~/.config/tokendog/originals/ and auto-expire after
TD_STASH_TTL seconds (default 24h).`,
}

var stashListCmd = &cobra.Command{
	Use:   "list",
	Short: "List stashed originals, newest first",
	RunE:  runStashList,
}

var stashGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Print the full original output for a stash id",
	Args:  cobra.ExactArgs(1),
	RunE:  runStashGet,
}

var stashPurgeCmd = &cobra.Command{
	Use:   "purge",
	Short: "Delete every stashed original",
	RunE:  runStashPurge,
}

func init() {
	stashCmd.AddCommand(stashListCmd)
	stashCmd.AddCommand(stashGetCmd)
	stashCmd.AddCommand(stashPurgeCmd)
}

func runStashList(_ *cobra.Command, _ []string) error {
	recs, err := stash.List()
	if err != nil {
		return err
	}
	if len(recs) == 0 {
		fmt.Println("No stashed originals. (Reversible compression off, or nothing large enough stashed yet.)")
		return nil
	}
	fmt.Printf("%-14s  %-19s  %10s  %s\n", "ID", "CREATED", "BYTES", "COMMAND")
	for _, r := range recs {
		cmd := r.Command
		if len(cmd) > 48 {
			cmd = cmd[:45] + "..."
		}
		fmt.Printf("%-14s  %-19s  %10d  %s\n",
			r.ID, r.CreatedAt.Format("2006-01-02 15:04:05"), r.OrigBytes, cmd)
	}
	return nil
}

func runStashGet(_ *cobra.Command, args []string) error {
	rec, ok := stash.Get(args[0])
	if !ok {
		return fmt.Errorf("no stashed output for id %q (expired or never existed)", args[0])
	}
	fmt.Print(rec.Content)
	if len(rec.Content) > 0 && rec.Content[len(rec.Content)-1] != '\n' {
		fmt.Println()
	}
	return nil
}

func runStashPurge(_ *cobra.Command, _ []string) error {
	n, err := stash.Purge()
	if err != nil {
		return err
	}
	fmt.Printf("Removed %d stashed original(s).\n", n)
	return nil
}
