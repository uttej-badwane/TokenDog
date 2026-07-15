package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// installedPlugins mirrors ~/.claude/plugins/installed_plugins.json
// (schema version 2): each "name@marketplace" maps to one or more
// install records. We only need where the plugin lives on disk.
type installedPlugins struct {
	Plugins map[string][]pluginInstall `json:"plugins"`
}

type pluginInstall struct {
	Scope       string `json:"scope"`
	InstallPath string `json:"installPath"`
	Version     string `json:"version"`
}

// auditPlugins audits installed Claude Code plugins. A plugin bundles
// its own commands/agents/skills/hooks/MCP under installPath, so it can
// carry the same risks as user config — a hook that pipes curl to a
// shell, a bundled MCP server with an inline secret — but it's easy to
// forget it's even there. We check the load-bearing, security-relevant
// pieces (manifest validity, bundled hooks, bundled MCP, secrets) and
// cross-check what's enabled against what's installed.
//
// Deep frontmatter linting of a plugin's vendored agents/commands is
// deliberately skipped: it's third-party content the user can't edit, so
// flagging it would be noise.
func (c *collector) auditPlugins(claudeHome string, enabled map[string]bool) {
	pluginsDir := filepath.Join(claudeHome, "plugins")
	indexPath := filepath.Join(pluginsDir, "installed_plugins.json")

	data, err := os.ReadFile(indexPath)
	if err != nil {
		// No plugins installed — still cross-check enabledPlugins, which
		// could reference something that was never installed.
		c.add(enabledNotInstalled(indexPath, enabled, nil)...)
		return
	}
	var idx installedPlugins
	if jsonErr := json.Unmarshal(data, &idx); jsonErr != nil {
		c.addItem(indexPath, "plugin", "plugin", false, jsonErr.Error())
		c.add(Finding{
			File: indexPath, Scope: "plugin", Dimension: "plugins", Severity: SeverityWarning,
			Issue: "installed_plugins.json is not valid JSON — Claude Code can't track plugins",
			Fix:   "repair the syntax (" + jsonErr.Error() + ")",
		})
		return
	}

	names := make([]string, 0, len(idx.Plugins))
	for name := range idx.Plugins {
		names = append(names, name)
	}
	sort.Strings(names)

	for i, name := range names {
		if i >= maxProjectDirs {
			break
		}
		records := idx.Plugins[name]
		if len(records) == 0 {
			continue
		}
		c.auditPlugin(name, records[0])
	}

	c.add(enabledNotInstalled(indexPath, enabled, idx.Plugins)...)
}

func (c *collector) auditPlugin(name string, rec pluginInstall) {
	root := rec.InstallPath
	if root == "" {
		return
	}
	c.addItem(root, "plugin", "plugin", true, "")

	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		c.add(Finding{
			File: root, Scope: "plugin", Dimension: "plugins", Severity: SeverityWarning,
			Issue: fmt.Sprintf("plugin %q is recorded as installed but its files are missing", name),
			Fix:   "reinstall or remove the plugin — Claude Code will fail to load it",
		})
		return
	}

	c.auditPluginManifest(name, root)
	c.auditPluginHooks(name, root)
	c.auditPluginMCP(name, root)
}

// auditPluginManifest validates <root>/.claude-plugin/plugin.json.
func (c *collector) auditPluginManifest(name, root string) {
	manifest := filepath.Join(root, ".claude-plugin", "plugin.json")
	data, tooBig, err := readCapped(manifest)
	if err != nil {
		c.add(Finding{
			File: root, Scope: "plugin", Dimension: "plugins", Severity: SeverityInfo,
			Issue: fmt.Sprintf("plugin %q has no .claude-plugin/plugin.json manifest", name),
			Fix:   "if the plugin misbehaves, reinstall it — a manifest is normally required to load",
		})
		return
	}
	if tooBig {
		return
	}
	var m map[string]any
	if jsonErr := json.Unmarshal(data, &m); jsonErr != nil {
		c.addItem(manifest, "plugin", "plugin", false, jsonErr.Error())
		c.add(Finding{
			File: manifest, Scope: "plugin", Dimension: "plugins", Severity: SeverityWarning,
			Issue: fmt.Sprintf("plugin %q manifest is not valid JSON", name),
			Fix:   "reinstall the plugin (" + jsonErr.Error() + ")",
		})
		return
	}
	c.addItem(manifest, "plugin", "plugin", true, "")
	if n, _ := m["name"].(string); n == "" {
		c.add(Finding{
			File: manifest, Scope: "plugin", Dimension: "plugins", Severity: SeverityWarning,
			Issue: fmt.Sprintf("plugin %q manifest is missing a name", name),
			Fix:   "reinstall the plugin — Claude Code keys plugins by their manifest name",
		})
	}
	c.add(scanSecrets(manifest, "plugin", data)...)
}

// auditPluginHooks runs the standard hook safety checks over a plugin's
// bundled hooks/hooks.json (same shape as settings.json's hooks block).
// Plugin hook commands use ${CLAUDE_PLUGIN_ROOT}, which the checker
// treats as unresolvable — so it skips path-existence but still flags
// pipe-to-shell and eval-of-substitution patterns.
func (c *collector) auditPluginHooks(name, root string) {
	hooksPath := filepath.Join(root, "hooks", "hooks.json")
	data, tooBig, err := readCapped(hooksPath)
	if err != nil || tooBig {
		return
	}
	var cfg map[string]any
	if jsonErr := json.Unmarshal(data, &cfg); jsonErr != nil {
		c.addItem(hooksPath, "plugin", "plugin", false, jsonErr.Error())
		c.add(Finding{
			File: hooksPath, Scope: "plugin", Dimension: "hooks", Severity: SeverityWarning,
			Issue: fmt.Sprintf("plugin %q hooks.json is not valid JSON", name),
			Fix:   "reinstall the plugin (" + jsonErr.Error() + ")",
		})
		return
	}
	c.addItem(hooksPath, "plugin", "plugin", true, "")
	for _, f := range analyzeHooks(hooksPath, "plugin", cfg, c.look) {
		f.Issue = fmt.Sprintf("plugin %q: %s", name, f.Issue)
		c.add(f)
	}
	c.add(scanSecrets(hooksPath, "plugin", data)...)
}

// auditPluginMCP evaluates a plugin's bundled .mcp.json, if any.
func (c *collector) auditPluginMCP(name, root string) {
	mcpPath := filepath.Join(root, ".mcp.json")
	data, tooBig, err := readCapped(mcpPath)
	if err != nil || tooBig {
		return
	}
	var cfg map[string]any
	if jsonErr := json.Unmarshal(data, &cfg); jsonErr != nil {
		c.addItem(mcpPath, "mcp", "plugin", false, jsonErr.Error())
		c.add(Finding{
			File: mcpPath, Scope: "plugin", Dimension: "mcp", Severity: SeverityWarning,
			Issue: fmt.Sprintf("plugin %q .mcp.json is not valid JSON", name),
			Fix:   "reinstall the plugin (" + jsonErr.Error() + ")",
		})
		return
	}
	c.addItem(mcpPath, "mcp", "plugin", true, "")
	servers, _ := cfg["mcpServers"].(map[string]any)
	c.add(analyzeMCPServers(mcpPath, "plugin", "plugin "+name, servers, c.look)...)
	c.add(scanSecrets(mcpPath, "plugin", data)...)
}

// enabledNotInstalled flags plugins turned on in settings.enabledPlugins
// that aren't present in installed_plugins.json — they silently do
// nothing. The enabled keys use the same "name@marketplace" form.
func enabledNotInstalled(indexPath string, enabled map[string]bool, installed map[string][]pluginInstall) []Finding {
	var missing []string
	for key, on := range enabled {
		if !on {
			continue
		}
		if _, ok := installed[key]; !ok {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	out := make([]Finding, 0, len(missing))
	for _, key := range missing {
		out = append(out, Finding{
			File: indexPath, Scope: "plugin", Dimension: "plugins", Severity: SeverityWarning,
			Issue: fmt.Sprintf("plugin %q is enabled but not installed — it has no effect", key),
			Fix:   "install it (/plugin) or remove it from enabledPlugins",
		})
	}
	return out
}

// enabledPluginKeys extracts the set of enabled plugin identifiers from a
// parsed settings map: enabledPlugins is {"name@marketplace": bool}.
func enabledPluginKeys(settings map[string]any) map[string]bool {
	out := map[string]bool{}
	ep, _ := settings["enabledPlugins"].(map[string]any)
	for key, v := range ep {
		if b, ok := v.(bool); ok && b {
			out[key] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
