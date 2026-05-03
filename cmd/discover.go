package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var discoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Find top unrewritten commands in your Claude history (missed savings)",
	RunE:  runDiscover,
}

type discoverStat struct {
	count     int
	bytes     int
	rewritten int
}

// supportedRewrites lists command names td knows how to filter.
// Keep this in sync with internal/hook/hook.go supported map.
var supportedRewrites = map[string]bool{
	"git":     true,
	"ls":      true,
	"find":    true,
	"docker":  true,
	"jq":      true,
	"curl":    true,
	"kubectl": true,
}

func runDiscover(_ *cobra.Command, _ []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	root := filepath.Join(home, ".claude", "projects")

	files, err := findJSONLFiles(root)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		fmt.Println("No Claude session files found at ~/.claude/projects/")
		return nil
	}

	stats := map[string]*discoverStat{}
	totalCmds := 0
	totalRewritten := 0

	for _, f := range files {
		if err := scanSession(f, stats, &totalCmds, &totalRewritten); err != nil {
			continue
		}
	}

	if totalCmds == 0 {
		fmt.Println("No Bash commands found in your Claude history.")
		return nil
	}

	type row struct {
		name string
		s    discoverStat
	}
	var rows []row
	for k, v := range stats {
		rows = append(rows, row{name: k, s: *v})
	}
	sort.Slice(rows, func(i, j int) bool {
		// Sort by missed count (count - rewritten), then total count
		mi := rows[i].s.count - rows[i].s.rewritten
		mj := rows[j].s.count - rows[j].s.rewritten
		if mi != mj {
			return mi > mj
		}
		return rows[i].s.count > rows[j].s.count
	})

	fmt.Println()
	fmt.Printf("Scanned %d session files, %d Bash commands\n", len(files), totalCmds)
	fmt.Printf("  Already through td:   %d (%.1f%%)\n", totalRewritten, pct(totalRewritten, totalCmds))
	fmt.Printf("  Direct (not via td):  %d\n", totalCmds-totalRewritten)
	fmt.Println()
	fmt.Println("Top commands (missed = ran directly without td)")
	fmt.Println(strings.Repeat("─", 78))
	fmt.Printf("  %-22s  %-7s  %-7s  %-9s  %s\n", "Command", "Total", "Missed", "Coverage", "Status")
	fmt.Println(strings.Repeat("─", 78))

	limit := 20
	for i, r := range rows {
		if i >= limit {
			break
		}
		missed := r.s.count - r.s.rewritten
		coverage := 100.0
		if r.s.count > 0 {
			coverage = float64(r.s.rewritten) / float64(r.s.count) * 100
		}
		status := "  not handled — open issue to add filter"
		if supportedRewrites[r.name] {
			status = "  supported by td  (check hook config)"
			if r.s.rewritten == r.s.count {
				status = "  ✓ fully covered"
			}
		}
		fmt.Printf("  %-22s  %-7d  %-7d  %6.1f%%   %s\n", trunc(r.name, 22), r.s.count, missed, coverage, status)
	}
	fmt.Println(strings.Repeat("─", 78))
	fmt.Println()
	return nil
}

func findJSONLFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".jsonl") {
			files = append(files, path)
		}
		return nil
	})
	if os.IsNotExist(err) {
		return nil, nil
	}
	return files, err
}

type sessionLine struct {
	Message struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type contentItem struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Input struct {
		Command string `json:"command"`
	} `json:"input"`
}

func scanSession(path string, stats map[string]*discoverStat, totalCmds, totalRewritten *int) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024) // big lines OK
	for scanner.Scan() {
		var sl sessionLine
		if err := json.Unmarshal(scanner.Bytes(), &sl); err != nil {
			continue
		}
		if len(sl.Message.Content) == 0 {
			continue
		}
		var items []contentItem
		if err := json.Unmarshal(sl.Message.Content, &items); err != nil {
			continue
		}
		for _, item := range items {
			if item.Type != "tool_use" || item.Name != "Bash" || item.Input.Command == "" {
				continue
			}
			cmd := strings.TrimSpace(item.Input.Command)
			binary, rewritten := analyzeCommand(cmd)
			if binary == "" {
				continue
			}
			*totalCmds++
			if rewritten {
				*totalRewritten++
			}
			s, ok := stats[binary]
			if !ok {
				s = &discoverStat{}
				stats[binary] = s
			}
			s.count++
			s.bytes += len(cmd)
			if rewritten {
				s.rewritten++
			}
		}
	}
	return scanner.Err()
}

// analyzeCommand returns the underlying binary name and whether the
// command is already routed through td.
func analyzeCommand(cmd string) (string, bool) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return "", false
	}
	first := parts[0]
	if first == "td" || first == "tokendog" {
		if len(parts) < 2 {
			return "", false
		}
		return parts[1], true
	}
	if idx := strings.LastIndex(first, "/"); idx >= 0 {
		first = first[idx+1:]
	}
	return first, false
}

func pct(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b) * 100
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
