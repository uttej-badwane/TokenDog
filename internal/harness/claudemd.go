package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// maxImportDepth matches Claude Code's own recursive-import limit.
const maxImportDepth = 5

// importRe matches Claude Code @imports: an @ at line start or after
// whitespace, followed by a path. Requiring a / or a .md suffix keeps
// decorations like "@channel" and npm scopes inside prose from matching;
// emails don't match because their @ follows a non-space character.
var importRe = regexp.MustCompile(`(?m)(?:^|\s)@([~A-Za-z0-9_][A-Za-z0-9_./~-]*)`)

// parseImports extracts @import paths from CLAUDE.md content, skipping
// fenced code blocks (Claude Code doesn't resolve imports inside them).
func parseImports(content []byte) []string {
	var out []string
	inFence := false
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		for _, m := range importRe.FindAllStringSubmatch(line, -1) {
			p := m[1]
			if strings.Contains(p, "/") || strings.HasSuffix(p, ".md") {
				out = append(out, p)
			}
		}
	}
	return out
}

// memoryLinkRe matches markdown links to local .md files in MEMORY.md —
// the index lines pointing at individual memory entries.
var memoryLinkRe = regexp.MustCompile(`\]\(([^)]+\.md)\)`)

// analyzeMemory cross-checks a memory dir's MEMORY.md index against the
// entry files actually present: linked-but-missing files are warnings
// (recall silently loses those memories), unlinked files and duplicate
// links are info (dead weight / noise in the index).
func analyzeMemory(dir, scope string) []Finding {
	indexPath := filepath.Join(dir, "MEMORY.md")
	data, tooBig, err := readCapped(indexPath)
	if err != nil || tooBig {
		return nil
	}

	linked := map[string]int{}
	for _, m := range memoryLinkRe.FindAllStringSubmatch(string(data), -1) {
		target := m[1]
		if strings.Contains(target, "://") || filepath.IsAbs(target) {
			continue // external or absolute link — not an index entry
		}
		linked[filepath.Clean(target)]++
	}

	present := map[string]bool{}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") && e.Name() != "MEMORY.md" {
			present[e.Name()] = true
		}
	}

	var names []string
	for name := range linked {
		names = append(names, name)
	}
	sort.Strings(names)

	var out []Finding
	for _, name := range names {
		if !present[filepath.Base(name)] {
			out = append(out, Finding{
				File: indexPath, Scope: scope, Dimension: "memory", Severity: SeverityWarning,
				Issue: fmt.Sprintf("index links %q but the file doesn't exist", name),
				Fix:   "remove the stale index line, or restore the memory file",
			})
		}
		if linked[name] > 1 {
			out = append(out, Finding{
				File: indexPath, Scope: scope, Dimension: "memory", Severity: SeverityInfo,
				Issue: fmt.Sprintf("index links %q %d times", name, linked[name]),
				Fix:   "keep a single index line per memory",
			})
		}
	}

	var orphans []string
	for name := range present {
		if linked[name] == 0 {
			orphans = append(orphans, name)
		}
	}
	sort.Strings(orphans)
	for _, name := range orphans {
		out = append(out, Finding{
			File: filepath.Join(dir, name), Scope: scope, Dimension: "memory", Severity: SeverityInfo,
			Issue: "memory file is not listed in MEMORY.md — it will never be recalled",
			Fix:   "add an index line to MEMORY.md, or delete the file",
		})
	}
	return out
}
