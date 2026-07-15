package harness

import (
	"sort"
	"time"
)

// Schema is the version of the JSON contract emitted by `td harness
// --json`. Bump when the shape changes incompatibly; the menu-bar app
// treats newer additive fields as optional.
const Schema = 1

// Finding is one issue detected by an analyzer. Issue text NEVER
// contains raw secret values — only pattern names and locations.
type Finding struct {
	File        string   `json:"file"`
	Issue       string   `json:"issue"`
	Severity    Severity `json:"severity"`
	Fix         string   `json:"fix"`
	Scope       string   `json:"scope"`     // "user" | "project" | "desktop"
	Dimension   string   `json:"dimension"` // settings, permissions, hooks, claudemd, memory, agents, commands, skills, mcp, keybindings, secrets, cross-scope
	AutoFixable bool     `json:"auto_fixable"`
	// FixID encodes everything `td harness apply` needs to perform the
	// fix (kind|path|details, see apply.go). Empty for report-only
	// findings.
	FixID string `json:"fix_id,omitempty"`
}

// Item is one inventoried config file.
type Item struct {
	Path       string    `json:"path"`
	Kind       string    `json:"kind"` // settings, claudemd, import, agent, command, skill, memory, mcp, keybindings, state
	Scope      string    `json:"scope"`
	SizeBytes  int64     `json:"size_bytes"`
	ModTime    time.Time `json:"mod_time"`
	Parses     bool      `json:"parses"`
	ParseError string    `json:"parse_error,omitempty"`
}

// Summary is the headline counts block.
type Summary struct {
	Critical     int `json:"critical"`
	Warning      int `json:"warning"`
	Info         int `json:"info"`
	AutoFixable  int `json:"auto_fixable"`
	FilesScanned int `json:"files_scanned"`
}

// Report is the full audit result — the stable, versioned contract
// behind `td harness --json`, consumed by the macOS menu-bar app.
type Report struct {
	Schema      int       `json:"schema"`
	GeneratedAt time.Time `json:"generated_at"`
	TDVersion   string    `json:"td_version"`
	ClaudeHome  string    `json:"claude_home"`
	ProjectRoot string    `json:"project_root,omitempty"`
	Summary     Summary   `json:"summary"`
	Inventory   []Item    `json:"inventory"`
	Findings    []Finding `json:"findings"`
}

func buildReport(opts Options, c *collector) *Report {
	findings := c.findings
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Severity != findings[j].Severity {
			return findings[i].Severity.rank() < findings[j].Severity.rank()
		}
		return findings[i].File < findings[j].File
	})

	var s Summary
	s.FilesScanned = len(c.items)
	for _, f := range findings {
		switch f.Severity {
		case SeverityCritical:
			s.Critical++
		case SeverityWarning:
			s.Warning++
		default:
			s.Info++
		}
		if f.AutoFixable {
			s.AutoFixable++
		}
	}

	// GeneratedAt truncated to whole seconds in UTC: Swift's .iso8601
	// decoder rejects fractional seconds (same constraint SpendReport
	// lives under).
	return &Report{
		Schema:      Schema,
		GeneratedAt: opts.Now().UTC().Truncate(time.Second),
		TDVersion:   opts.Version,
		ClaudeHome:  opts.ClaudeHome,
		ProjectRoot: opts.ProjectRoot,
		Summary:     s,
		Inventory:   c.items,
		Findings:    findings,
	}
}
