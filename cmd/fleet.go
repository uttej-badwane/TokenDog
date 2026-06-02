package cmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
	"tokendog/internal/analytics"
	"tokendog/internal/policy"
)

var fleetCmd = &cobra.Command{
	Use:   "fleet",
	Short: "Fleet observability + centrally-managed policy for platform teams",
	Long: `Turns the per-laptop tool into something a platform team can run across a
fleet:

  td fleet push --endpoint https://collector.internal/tokendog   # report savings
  td fleet pull https://config.internal/tokendog/policy.json      # fetch policy
  td fleet policy                                                 # show effective policy

Reporting is opt-in and privacy-preserving: the push payload contains only
aggregate counts, bytes, and tokens plus a hashed machine id — never command
strings, arguments, or any tool output.`,
}

var (
	fleetPushEndpoint string
	fleetPushSince    string
	fleetPushDryRun   bool
)

var fleetPushCmd = &cobra.Command{
	Use:   "push",
	Short: "Report aggregate savings to an internal collector (opt-in, no content)",
	RunE:  runFleetPush,
}

var fleetPullCmd = &cobra.Command{
	Use:   "pull <url>",
	Short: "Fetch a managed policy and install it locally",
	Args:  cobra.ExactArgs(1),
	RunE:  runFleetPull,
}

var fleetPolicyCmd = &cobra.Command{
	Use:   "policy",
	Short: "Show the effective managed policy",
	RunE:  runFleetPolicy,
}

func init() {
	fleetPushCmd.Flags().StringVar(&fleetPushEndpoint, "endpoint", "", "Collector URL to POST the report to (required)")
	fleetPushCmd.Flags().StringVar(&fleetPushSince, "since", "", "Only include records on or after this date (YYYY-MM-DD or 7d/2w/1m/1y)")
	fleetPushCmd.Flags().BoolVar(&fleetPushDryRun, "dry-run", false, "Print the payload without sending it")
	fleetCmd.AddCommand(fleetPushCmd)
	fleetCmd.AddCommand(fleetPullCmd)
	fleetCmd.AddCommand(fleetPolicyCmd)
}

// fleetReport is the privacy-preserving payload. Deliberately carries no
// command strings, arguments, or tool output — only aggregates and a hashed,
// non-reversible machine id so a collector can count distinct machines without
// identifying them.
type fleetReport struct {
	Schema        string    `json:"schema"`
	MachineID     string    `json:"machine_id"`
	GeneratedAt   time.Time `json:"generated_at"`
	Since         string    `json:"since,omitempty"`
	Commands      int       `json:"commands"`
	RawBytes      int       `json:"raw_bytes"`
	FilteredBytes int       `json:"filtered_bytes"`
	BytesSaved    int       `json:"bytes_saved"`
	TokensSaved   int       `json:"tokens_saved"`
	CacheHits     int       `json:"cache_hits"`
	TDVersion     string    `json:"td_version"`
}

func runFleetPush(_ *cobra.Command, _ []string) error {
	if fleetPushEndpoint == "" && !fleetPushDryRun {
		return fmt.Errorf("--endpoint is required (or use --dry-run to preview the payload)")
	}

	records, err := analytics.LoadAll()
	if err != nil {
		return err
	}
	if fleetPushSince != "" {
		cutoff, err := parseDateOrDuration(fleetPushSince)
		if err != nil {
			return fmt.Errorf("--since %q: %w", fleetPushSince, err)
		}
		var kept []analytics.Record
		for _, r := range records {
			if !cutoff.IsZero() && r.Timestamp.Before(cutoff) {
				continue
			}
			kept = append(kept, r)
		}
		records = kept
	}

	sum, _ := analytics.Summarize(records)
	report := fleetReport{
		Schema:        "tokendog.fleet.v1",
		MachineID:     machineID(),
		GeneratedAt:   time.Now().UTC(),
		Since:         fleetPushSince,
		Commands:      sum.TotalCommands,
		RawBytes:      sum.TotalRawBytes,
		FilteredBytes: sum.TotalFilteredBytes,
		BytesSaved:    sum.TotalRawBytes - sum.TotalFilteredBytes,
		TokensSaved:   sum.TotalTokensSaved,
		CacheHits:     sum.CacheHits,
		TDVersion:     Version,
	}

	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}

	if fleetPushDryRun {
		fmt.Println(string(payload))
		fmt.Println("\n(dry-run; nothing sent. Payload carries no command strings or output.)")
		return nil
	}

	resp, err := http.Post(fleetPushEndpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("posting report: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("collector returned %s", resp.Status)
	}
	fmt.Printf("Reported %d commands (%d tokens saved) to %s [%s]\n",
		report.Commands, report.TokensSaved, fleetPushEndpoint, resp.Status)
	return nil
}

func runFleetPull(_ *cobra.Command, args []string) error {
	url := args[0]
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("fetching policy: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("policy URL returned %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	// Validate before persisting — a malformed policy must not be installed.
	var p policy.Policy
	if err := json.Unmarshal(data, &p); err != nil {
		return fmt.Errorf("policy is not valid JSON: %w", err)
	}
	if err := policy.Save(p); err != nil {
		return err
	}
	path, _ := policy.Path()
	fmt.Printf("Installed managed policy to %s\n", path)
	printPolicy(p)
	return nil
}

func runFleetPolicy(_ *cobra.Command, _ []string) error {
	p := policy.Load()
	if p.Empty() {
		fmt.Println("No managed policy installed — engine uses built-in defaults (dedup on, reversible off).")
		fmt.Println("A platform team can install one with: td fleet pull <url>")
		return nil
	}
	path, _ := policy.Path()
	fmt.Printf("Managed policy (%s):\n", path)
	printPolicy(p)
	fmt.Println("\nNote: an explicitly-set env var (TD_NO_DEDUP / TD_REVERSIBLE / TD_STASH_MIN) overrides policy locally.")
	return nil
}

func printPolicy(p policy.Policy) {
	show := func(name string, set bool, val string) {
		if set {
			fmt.Printf("  %-14s %s\n", name, val)
		} else {
			fmt.Printf("  %-14s (unset → default)\n", name)
		}
	}
	show("dedup", p.Dedup != nil, boolStr(p.Dedup))
	show("reversible", p.Reversible != nil, boolStr(p.Reversible))
	if p.StashMinBytes != nil {
		fmt.Printf("  %-14s %d\n", "stash_min", *p.StashMinBytes)
	} else {
		fmt.Printf("  %-14s (unset → default)\n", "stash_min")
	}
}

func boolStr(b *bool) string {
	if b == nil {
		return ""
	}
	if *b {
		return "on"
	}
	return "off"
}

// machineID is a stable, non-reversible identifier so a collector can count
// distinct machines without learning the hostname.
func machineID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown-host"
	}
	sum := sha256.Sum256([]byte("tokendog-fleet|" + host))
	return hex.EncodeToString(sum[:])[:16]
}
