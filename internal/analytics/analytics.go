package analytics

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Record struct {
	Command       string    `json:"command"`
	Timestamp     time.Time `json:"timestamp"`
	RawBytes      int       `json:"raw_bytes"`
	FilteredBytes int       `json:"filtered_bytes"`
	DurationMs    int64     `json:"duration_ms"`
}

func (r Record) BytesSaved() int { return r.RawBytes - r.FilteredBytes }

func (r Record) SavedPct() float64 {
	if r.RawBytes == 0 {
		return 0
	}
	return float64(r.BytesSaved()) / float64(r.RawBytes) * 100
}

func EstimateTokens(bytes int) int { return (bytes + 3) / 4 }

func dataPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".config", "tokendog")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "history.jsonl"), nil
}

func Save(r Record) error {
	path, err := dataPath()
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(r)
}

func LoadAll() ([]Record, error) {
	path, err := dataPath()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var records []Record
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var r Record
		if json.Unmarshal(scanner.Bytes(), &r) == nil {
			records = append(records, r)
		}
	}
	return records, scanner.Err()
}

type Summary struct {
	TotalCommands     int
	TotalRawBytes     int
	TotalFilteredBytes int
	TotalDurationMs   int64
}

func (s Summary) BytesSaved() int { return s.TotalRawBytes - s.TotalFilteredBytes }
func (s Summary) TokensSaved() int { return EstimateTokens(s.BytesSaved()) }
func (s Summary) SavedPct() float64 {
	if s.TotalRawBytes == 0 {
		return 0
	}
	return float64(s.BytesSaved()) / float64(s.TotalRawBytes) * 100
}

type CommandStat struct {
	Name   string
	Count  int
	Saved  int
	AvgPct float64
	AvgMs  int64
}

func Summarize(records []Record) (Summary, []CommandStat) {
	var s Summary
	byCmd := map[string]*CommandStat{}

	for _, r := range records {
		s.TotalCommands++
		s.TotalRawBytes += r.RawBytes
		s.TotalFilteredBytes += r.FilteredBytes
		s.TotalDurationMs += r.DurationMs

		name := normalizeName(r.Command)
		cs, ok := byCmd[name]
		if !ok {
			cs = &CommandStat{Name: name}
			byCmd[name] = cs
		}
		cs.Count++
		cs.Saved += r.BytesSaved()
		cs.AvgPct = (cs.AvgPct*float64(cs.Count-1) + r.SavedPct()) / float64(cs.Count)
		cs.AvgMs = (cs.AvgMs*int64(cs.Count-1) + r.DurationMs) / int64(cs.Count)
	}

	stats := make([]CommandStat, 0, len(byCmd))
	for _, cs := range byCmd {
		stats = append(stats, *cs)
	}
	sort.Slice(stats, func(i, j int) bool { return stats[i].Saved > stats[j].Saved })
	return s, stats
}

func normalizeName(cmd string) string {
	cmd = strings.TrimPrefix(cmd, "td ")
	cmd = strings.TrimPrefix(cmd, "tokendog ")
	parts := strings.Fields(cmd)
	if len(parts) >= 2 {
		return parts[0] + " " + parts[1]
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return cmd
}

func RenderGain(records []Record, showHistory bool) string {
	if len(records) == 0 {
		return "No data yet. Run td commands to start tracking savings.\n"
	}

	summary, stats := Summarize(records)
	var b strings.Builder

	sep60 := strings.Repeat("═", 60)
	sep71 := strings.Repeat("─", 71)

	b.WriteString("TokenDog Savings\n")
	b.WriteString(sep60 + "\n\n")
	b.WriteString(fmt.Sprintf("%-22s %d\n", "Total commands:", summary.TotalCommands))
	b.WriteString(fmt.Sprintf("%-22s %s\n", "Raw output:", humanBytes(summary.TotalRawBytes)))
	b.WriteString(fmt.Sprintf("%-22s %s\n", "After filter:", humanBytes(summary.TotalFilteredBytes)))
	b.WriteString(fmt.Sprintf("%-22s %s (~%d tokens, %.1f%%)\n",
		"Saved:", humanBytes(summary.BytesSaved()), summary.TokensSaved(), summary.SavedPct()))

	pct := summary.SavedPct()
	b.WriteString(fmt.Sprintf("%-22s %s %.1f%%\n\n", "Efficiency:", progressBar(pct, 24), pct))

	b.WriteString("By Command\n")
	b.WriteString(sep71 + "\n")
	b.WriteString(fmt.Sprintf("  %-3s  %-28s  %-5s  %-8s  %-6s  %-6s  %s\n",
		"#", "Command", "Count", "Saved", "Avg%", "AvgMs", "Impact"))
	b.WriteString(sep71 + "\n")

	maxSaved := 0
	for _, cs := range stats {
		if cs.Saved > maxSaved {
			maxSaved = cs.Saved
		}
	}

	for i, cs := range stats {
		impact := ""
		if maxSaved > 0 {
			impact = progressBar(float64(cs.Saved)/float64(maxSaved)*100, 10)
		}
		name := cs.Name
		if len(name) > 28 {
			name = name[:25] + "..."
		}
		b.WriteString(fmt.Sprintf("  %-3d  %-28s  %-5d  %-8s  %5.1f%%  %4dms  %s\n",
			i+1, name, cs.Count, humanBytes(cs.Saved), cs.AvgPct, cs.AvgMs, impact))
	}
	b.WriteString(sep71 + "\n")

	if showHistory && len(records) > 0 {
		b.WriteString("\nRecent Commands\n")
		b.WriteString(strings.Repeat("─", 60) + "\n")
		start := len(records) - 20
		if start < 0 {
			start = 0
		}
		for _, r := range records[start:] {
			arrow := "•"
			if r.BytesSaved() > 0 {
				arrow = "▲"
			}
			name := r.Command
			if len(name) > 32 {
				name = name[:29] + "..."
			}
			b.WriteString(fmt.Sprintf("%s %s %-34s %4.0f%% (%s)\n",
				r.Timestamp.Format("01-02 15:04"), arrow, name, r.SavedPct(), humanBytes(r.BytesSaved())))
		}
	}

	return b.String()
}

func progressBar(pct float64, width int) string {
	filled := int(pct / 100 * float64(width))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func humanBytes(n int) string {
	switch {
	case n >= 1024*1024:
		return fmt.Sprintf("%.1fMB", float64(n)/1024/1024)
	case n >= 1024:
		return fmt.Sprintf("%.1fKB", float64(n)/1024)
	default:
		return fmt.Sprintf("%dB", n)
	}
}
