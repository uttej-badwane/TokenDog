package harness

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"tokendog/internal/mcpconfig"
)

const (
	// maxWalkFiles caps directory walks so a pathological commands/ or
	// memory/ tree can't stall the audit.
	maxWalkFiles = 2000
	// maxScanBytes caps per-file content analysis (secret scan, import
	// resolution). Bigger files are inventoried but not content-scanned.
	maxScanBytes = 2 << 20
	// maxProjectDirs caps how many ~/.claude/projects/<slug> children we
	// probe for memory/ subdirs.
	maxProjectDirs = 200
)

// collector accumulates inventory items and findings across scopes.
type collector struct {
	items    []Item
	findings []Finding
	look     func(string) (string, error)

	userSettingsPath string
	projSettingsPath string

	// name → path per scope, for user-vs-project collision detection.
	userCommands, projCommands map[string]string
	userSkills, projSkills     map[string]string
}

func (c *collector) add(f ...Finding) { c.findings = append(c.findings, f...) }

// addItem stats path and records an inventory entry. Silently skips
// paths that vanished between listing and stat.
func (c *collector) addItem(path, kind, scope string, parses bool, parseErr string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	c.items = append(c.items, Item{
		Path:       path,
		Kind:       kind,
		Scope:      scope,
		SizeBytes:  info.Size(),
		ModTime:    info.ModTime().UTC().Truncate(time.Second),
		Parses:     parses,
		ParseError: parseErr,
	})
}

// readCapped reads at most maxScanBytes of path. tooBig reports that the
// file was larger and the content is truncated (callers should skip
// content analysis and note it).
func readCapped(path string) (data []byte, tooBig bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, false, err
	}
	if info.Size() > maxScanBytes {
		return nil, true, nil
	}
	data, err = os.ReadFile(path)
	return data, false, err
}

// auditScope inventories one scope: settings(.local).json + agents +
// commands + skills under claudeDir, and CLAUDE.md under mdDir (for the
// user scope both are ~/.claude; for a project, claudeDir is
// <root>/.claude and CLAUDE.md sits at the root). Returns the parsed
// settings.json map for cross-scope analysis (nil if absent/invalid).
func (c *collector) auditScope(scope, claudeDir, mdDir string) map[string]any {
	settings := c.auditSettingsFile(scope, filepath.Join(claudeDir, "settings.json"))
	c.auditSettingsFile(scope, filepath.Join(claudeDir, "settings.local.json"))
	if scope == "user" {
		c.userSettingsPath = filepath.Join(claudeDir, "settings.json")
	} else {
		c.projSettingsPath = filepath.Join(claudeDir, "settings.json")
	}

	c.auditClaudeMD(scope, filepath.Join(mdDir, "CLAUDE.md"))
	// Projects sometimes keep CLAUDE.md inside .claude/ as well.
	if mdDir != claudeDir {
		c.auditClaudeMD(scope, filepath.Join(claudeDir, "CLAUDE.md"))
	}

	c.auditAgents(scope, filepath.Join(claudeDir, "agents"))
	commands := c.auditCommands(scope, filepath.Join(claudeDir, "commands"))
	skills := c.auditSkills(scope, filepath.Join(claudeDir, "skills"))
	if scope == "user" {
		c.userCommands, c.userSkills = commands, skills
	} else {
		c.projCommands, c.projSkills = commands, skills
	}
	return settings
}

// auditSettingsFile reads and analyzes one settings file. Missing file →
// nothing recorded (absence is normal). Invalid JSON → inventory item
// with Parses=false plus a warning finding.
func (c *collector) auditSettingsFile(scope, path string) map[string]any {
	data, tooBig, err := readCapped(path)
	if err != nil {
		return nil
	}
	if tooBig {
		c.addItem(path, "settings", scope, false, "file exceeds scan cap")
		c.add(Finding{
			File: path, Scope: scope, Dimension: "settings", Severity: SeverityWarning,
			Issue: fmt.Sprintf("file is larger than %d MB — skipped content checks", maxScanBytes>>20),
			Fix:   "a settings file this large is almost certainly wrong; inspect it manually",
		})
		return nil
	}
	var cfg map[string]any
	if jsonErr := json.Unmarshal(data, &cfg); jsonErr != nil {
		c.addItem(path, "settings", scope, false, jsonErr.Error())
		c.add(Finding{
			File: path, Scope: scope, Dimension: "settings", Severity: SeverityWarning,
			Issue: "not valid JSON — Claude Code will ignore this file",
			Fix:   "repair the syntax (" + jsonErr.Error() + ")",
		})
		return nil
	}
	c.addItem(path, "settings", scope, true, "")
	c.add(analyzeSettingsMap(path, scope, cfg)...)
	c.add(analyzePermissions(path, scope, cfg)...)
	c.add(analyzeHooks(path, scope, cfg, c.look)...)
	c.add(scanSecrets(path, scope, data)...)
	return cfg
}

// auditClaudeMD inventories a CLAUDE.md and everything it @imports,
// recursively (cycle-safe, depth-capped).
func (c *collector) auditClaudeMD(scope, path string) {
	visited := map[string]bool{}
	c.claudeMDFile(scope, path, "claudemd", 0, visited)
}

func (c *collector) claudeMDFile(scope, path, kind string, depth int, visited map[string]bool) {
	abs, err := filepath.Abs(path)
	if err != nil || visited[abs] {
		return
	}
	visited[abs] = true

	data, tooBig, err := readCapped(path)
	if err != nil {
		return
	}
	c.addItem(path, kind, scope, true, "")
	if tooBig {
		c.add(Finding{
			File: path, Scope: scope, Dimension: "claudemd", Severity: SeverityWarning,
			Issue: fmt.Sprintf("file exceeds %d MB — every token of it is loaded into context each session", maxScanBytes>>20),
			Fix:   "trim it; move reference material into skills or on-demand files",
		})
		return
	}
	c.add(scanSecrets(path, scope, data)...)

	for _, imp := range parseImports(data) {
		target := imp
		if strings.HasPrefix(target, "~") {
			target = expandHome(target)
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(path), target)
		}
		if _, statErr := os.Stat(target); statErr != nil {
			c.add(Finding{
				File: path, Scope: scope, Dimension: "claudemd", Severity: SeverityWarning,
				Issue: fmt.Sprintf("@import %q does not resolve (%s)", imp, Tildify(target)),
				Fix:   "fix the path or remove the import — Claude Code silently drops broken imports",
			})
			continue
		}
		if depth >= maxImportDepth {
			c.add(Finding{
				File: path, Scope: scope, Dimension: "claudemd", Severity: SeverityInfo,
				Issue: fmt.Sprintf("@import %q exceeds the %d-level import depth Claude Code resolves", imp, maxImportDepth),
				Fix:   "flatten the import chain",
			})
			continue
		}
		c.claudeMDFile(scope, target, "import", depth+1, visited)
	}
}

// auditAgents inventories <dir>/*.md agent definitions.
func (c *collector) auditAgents(scope, dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, tooBig, err := readCapped(path)
		if err != nil || tooBig {
			continue
		}
		parses, parseErr, findings := analyzeAgentFile(path, scope, data)
		c.addItem(path, "agent", scope, parses, parseErr)
		c.add(findings...)
		c.add(scanSecrets(path, scope, data)...)
	}
}

// auditCommands walks <dir>/**/*.md slash-command definitions and
// returns name→path for collision detection. Command names are the
// relative path minus the .md extension (dirs become : namespaces in
// Claude Code, but the relative path is enough for shadow detection).
func (c *collector) auditCommands(scope, dir string) map[string]string {
	names := map[string]string{}
	count := 0
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if count >= maxWalkFiles {
			return fs.SkipAll
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		count++
		rel, _ := filepath.Rel(dir, path)
		names[strings.TrimSuffix(filepath.ToSlash(rel), ".md")] = path

		data, tooBig, readErr := readCapped(path)
		if readErr != nil || tooBig {
			return nil
		}
		// Frontmatter is optional for commands, but if a block is opened
		// it must be closed or Claude Code misparses the whole file.
		if _, present, fmErr := parseFrontmatter(data); present && fmErr != nil {
			c.addItem(path, "command", scope, false, fmErr.Error())
			c.add(Finding{
				File: path, Scope: scope, Dimension: "commands", Severity: SeverityWarning,
				Issue: "frontmatter block is malformed (" + fmErr.Error() + ")",
				Fix:   "close the --- fence or remove the block",
			})
		} else {
			c.addItem(path, "command", scope, true, "")
		}
		c.add(scanSecrets(path, scope, data)...)
		return nil
	})
	return names
}

// auditSkills inventories <dir>/<name>/SKILL.md files and returns
// name→path for collision detection.
func (c *collector) auditSkills(scope, dir string) map[string]string {
	names := map[string]string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return names
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillMD := filepath.Join(dir, e.Name(), "SKILL.md")
		data, tooBig, readErr := readCapped(skillMD)
		if readErr != nil {
			c.add(Finding{
				File: filepath.Join(dir, e.Name()), Scope: scope, Dimension: "skills", Severity: SeverityWarning,
				Issue: "skill directory has no SKILL.md — Claude Code won't load it",
				Fix:   "add a SKILL.md with name/description frontmatter, or delete the directory",
			})
			continue
		}
		names[e.Name()] = skillMD
		if tooBig {
			c.addItem(skillMD, "skill", scope, true, "")
			continue
		}
		parses, parseErr, findings := analyzeSkillFile(skillMD, scope, data)
		c.addItem(skillMD, "skill", scope, parses, parseErr)
		c.add(findings...)
		c.add(scanSecrets(skillMD, scope, data)...)
	}
	return names
}

// auditKeybindings inventories ~/.claude/keybindings.json (user scope
// only — Claude Code has no project-level keybindings).
func (c *collector) auditKeybindings(claudeHome string) {
	path := filepath.Join(claudeHome, "keybindings.json")
	data, tooBig, err := readCapped(path)
	if err != nil || tooBig {
		return
	}
	parses, parseErr, findings := analyzeKeybindings(path, data)
	c.addItem(path, "keybindings", "user", parses, parseErr)
	c.add(findings...)
	c.add(scanSecrets(path, "user", data)...)
}

// auditMemoryDirs finds memory directories: ~/.claude/memory plus each
// ~/.claude/projects/<slug>/memory (Claude Code's per-project
// auto-memory). Probing direct children is cheap; we never walk into the
// transcript .jsonl trees.
func (c *collector) auditMemoryDirs(claudeHome string) {
	c.auditMemoryDir(filepath.Join(claudeHome, "memory"))
	children, err := os.ReadDir(filepath.Join(claudeHome, "projects"))
	if err != nil {
		return
	}
	for i, child := range children {
		if i >= maxProjectDirs {
			break
		}
		if !child.IsDir() {
			continue
		}
		c.auditMemoryDir(filepath.Join(claudeHome, "projects", child.Name(), "memory"))
	}
}

func (c *collector) auditMemoryDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		c.addItem(path, "memory", "user", true, "")
		if data, tooBig, readErr := readCapped(path); readErr == nil && !tooBig {
			c.add(scanSecrets(path, "user", data)...)
		}
	}
	c.add(analyzeMemory(dir, "user")...)
}

// auditProjectMCP inventories <root>/.mcp.json.
func (c *collector) auditProjectMCP(root string) {
	path := filepath.Join(root, ".mcp.json")
	data, tooBig, err := readCapped(path)
	if err != nil || tooBig {
		return
	}
	var cfg map[string]any
	if jsonErr := json.Unmarshal(data, &cfg); jsonErr != nil {
		c.addItem(path, "mcp", "project", false, jsonErr.Error())
		c.add(Finding{
			File: path, Scope: "project", Dimension: "mcp", Severity: SeverityWarning,
			Issue: "not valid JSON — Claude Code will ignore these MCP servers",
			Fix:   "repair the syntax (" + jsonErr.Error() + ")",
		})
		return
	}
	c.addItem(path, "mcp", "project", true, "")
	servers, _ := cfg["mcpServers"].(map[string]any)
	c.add(analyzeMCPServers(path, "project", "", servers, c.look)...)
	c.add(scanSecrets(path, "project", data)...)
}

// auditUserMCPJSON inventories ~/.claude/mcp.json — a user-scope MCP
// config file (same mcpServers shape as a project's .mcp.json). Distinct
// from ~/.claude.json (Claude Code's state file), which auditClaudeJSON
// handles.
func (c *collector) auditUserMCPJSON(claudeHome string) {
	path := filepath.Join(claudeHome, "mcp.json")
	data, tooBig, err := readCapped(path)
	if err != nil || tooBig {
		return
	}
	var cfg map[string]any
	if jsonErr := json.Unmarshal(data, &cfg); jsonErr != nil {
		c.addItem(path, "mcp", "user", false, jsonErr.Error())
		c.add(Finding{
			File: path, Scope: "user", Dimension: "mcp", Severity: SeverityWarning,
			Issue: "not valid JSON — Claude Code will ignore these MCP servers",
			Fix:   "repair the syntax (" + jsonErr.Error() + ")",
		})
		return
	}
	c.addItem(path, "mcp", "user", true, "")
	servers, _ := cfg["mcpServers"].(map[string]any)
	c.add(analyzeMCPServers(path, "user", "", servers, c.look)...)
	c.add(scanSecrets(path, "user", data)...)
}

// auditClaudeJSON inspects ~/.claude.json — Claude Code's state file.
// It can be many MB of per-project history, so we parse it (validity +
// mcpServers extraction) but deliberately do NOT secret-scan the whole
// file: its history subtrees would drown the report in noise. The MCP
// env check covers the part that matters.
//
// MCP servers live in two places here: a top-level mcpServers map
// (global servers) and a per-project mcpServers block under
// projects.<abs-path> (the servers `claude mcp add` writes for a given
// directory). Both are evaluated; per-project servers are labelled with
// their project path.
func (c *collector) auditClaudeJSON(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var cfg map[string]any
	if jsonErr := json.Unmarshal(data, &cfg); jsonErr != nil {
		c.addItem(path, "state", "user", false, jsonErr.Error())
		c.add(Finding{
			File: path, Scope: "user", Dimension: "mcp", Severity: SeverityWarning,
			Issue: "not valid JSON — Claude Code state (including MCP servers) is unreadable",
			Fix:   "repair the syntax; Claude Code may regenerate it on next launch",
		})
		return
	}
	c.addItem(path, "state", "user", true, "")

	servers, _ := cfg["mcpServers"].(map[string]any)
	c.add(analyzeMCPServers(path, "user", "", servers, c.look)...)

	// Per-project servers: projects.<abs-path>.mcpServers. Sorted by
	// project path for deterministic output.
	projects, _ := cfg["projects"].(map[string]any)
	projPaths := make([]string, 0, len(projects))
	for p := range projects {
		projPaths = append(projPaths, p)
	}
	sort.Strings(projPaths)
	for _, p := range projPaths {
		pcfg, _ := projects[p].(map[string]any)
		psrv, _ := pcfg["mcpServers"].(map[string]any)
		if len(psrv) == 0 {
			continue
		}
		c.add(analyzeMCPServers(path, "user", "project "+Tildify(p), psrv, c.look)...)
	}
}

// auditDesktopConfig inventories Claude Desktop's
// claude_desktop_config.json (path logic shared with `td mcp` via
// mcpconfig). Missing file or unsupported OS is simply not a finding.
func (c *collector) auditDesktopConfig() {
	path, err := mcpconfig.ConfigPath()
	if err != nil {
		return
	}
	data, tooBig, err := readCapped(path)
	if err != nil || tooBig {
		return
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		c.addItem(path, "mcp", "desktop", true, "")
		return
	}
	var cfg map[string]any
	if jsonErr := json.Unmarshal(data, &cfg); jsonErr != nil {
		c.addItem(path, "mcp", "desktop", false, jsonErr.Error())
		c.add(Finding{
			File: path, Scope: "desktop", Dimension: "mcp", Severity: SeverityWarning,
			Issue: "not valid JSON — Claude Desktop will ignore its MCP servers",
			Fix:   "repair the syntax (" + jsonErr.Error() + ")",
		})
		return
	}
	c.addItem(path, "mcp", "desktop", true, "")
	servers, _ := cfg["mcpServers"].(map[string]any)
	c.add(analyzeMCPServers(path, "desktop", "", servers, c.look)...)
	c.add(scanSecrets(path, "desktop", data)...)
}

// collisionFindings reports project commands/skills that shadow a
// same-named user one. Sorted for deterministic output.
func (c *collector) collisionFindings() []Finding {
	var out []Finding
	out = append(out, collisions("commands", "command", c.userCommands, c.projCommands)...)
	out = append(out, collisions("skills", "skill", c.userSkills, c.projSkills)...)
	return out
}

func collisions(dimension, noun string, user, proj map[string]string) []Finding {
	var names []string
	for name := range proj {
		if _, ok := user[name]; ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	out := make([]Finding, 0, len(names))
	for _, name := range names {
		out = append(out, Finding{
			File: proj[name], Scope: "project", Dimension: dimension, Severity: SeverityInfo,
			Issue: fmt.Sprintf("project %s %q shadows the user-level %s of the same name", noun, name, noun),
			Fix:   "rename one of them if the shadowing is unintentional",
		})
	}
	return out
}
