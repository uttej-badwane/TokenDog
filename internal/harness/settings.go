package harness

import (
	"fmt"
	"sort"
)

// knownSettingsKeys is the curated allow-list of top-level keys Claude
// Code reads from settings.json. Unknown keys are flagged at info level
// only — the list is best-effort and new releases add keys — but a flag
// here catches the classic silent typo ("permisions", "hook").
var knownSettingsKeys = map[string]bool{
	"$schema":                    true,
	"alwaysThinkingEnabled":      true,
	"apiKeyHelper":               true,
	"autoUpdates":                true,
	"awsAuthRefresh":             true,
	"awsCredentialExport":        true,
	"cleanupPeriodDays":          true,
	"companyAnnouncements":       true,
	"disableAllHooks":            true,
	"disabledMcpjsonServers":     true,
	"enableAllProjectMcpServers": true,
	"enabledMcpjsonServers":      true,
	"enabledPlugins":             true,
	"env":                        true,
	"extraKnownMarketplaces":     true,
	"forceLoginMethod":           true,
	"forceLoginOrgUUID":          true,
	"hooks":                      true,
	"includeCoAuthoredBy":        true,
	"language":                   true,
	"model":                      true,
	"otelHeadersHelper":          true,
	"outputStyle":                true,
	"permissions":                true,
	"sandbox":                    true,
	"spinnerTipsEnabled":         true,
	"statusLine":                 true,
}

// deprecatedSettingsKeys maps keys Claude Code no longer honors to the
// modern replacement.
var deprecatedSettingsKeys = map[string]string{
	"allowedTools":      "move the rules to permissions.allow",
	"ignorePatterns":    "move to permissions.deny with Read() rules",
	"autoUpdaterStatus": "use autoUpdates (boolean)",
}

// analyzeSettingsMap flags unknown and deprecated top-level keys.
func analyzeSettingsMap(path, scope string, cfg map[string]any) []Finding {
	var keys []string
	for k := range cfg {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var out []Finding
	for _, k := range keys {
		if replacement, dep := deprecatedSettingsKeys[k]; dep {
			out = append(out, Finding{
				File: path, Scope: scope, Dimension: "settings", Severity: SeverityWarning,
				Issue: fmt.Sprintf("key %q is deprecated and ignored by current Claude Code", k),
				Fix:   replacement,
			})
			continue
		}
		if !knownSettingsKeys[k] {
			out = append(out, Finding{
				File: path, Scope: scope, Dimension: "settings", Severity: SeverityInfo,
				Issue: fmt.Sprintf("key %q is not a recognized Claude Code setting (typo, or newer than this check)", k),
				Fix:   "verify the spelling against the Claude Code settings docs; unknown keys are silently ignored",
			})
		}
	}
	return out
}
