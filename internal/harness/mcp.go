package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"tokendog/internal/redact"
)

// analyzeMCPServers checks the mcpServers map from one config source:
// stdio servers whose command can't be resolved (warning — the server
// silently fails to start), entries with neither command nor url
// (warning), and env values that embed a recognizable secret (critical).
// Connectivity is deliberately not tested — the audit stays offline.
func analyzeMCPServers(path, scope string, servers map[string]any, look func(string) (string, error)) []Finding {
	if len(servers) == 0 {
		return nil
	}
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)

	var out []Finding
	for _, name := range names {
		entry, _ := servers[name].(map[string]any)
		if entry == nil {
			continue
		}
		command, _ := entry["command"].(string)
		url, _ := entry["url"].(string)

		switch {
		case command == "" && url == "":
			out = append(out, Finding{
				File: path, Scope: scope, Dimension: "mcp", Severity: SeverityWarning,
				Issue: fmt.Sprintf("server %q has neither a command nor a url", name),
				Fix:   "add a command (stdio) or url (http/sse), or remove the entry",
			})
		case command != "":
			if missing := commandMissing(command, look); missing != "" {
				out = append(out, Finding{
					File: path, Scope: scope, Dimension: "mcp", Severity: SeverityWarning,
					Issue: fmt.Sprintf("server %q: %s", name, missing),
					Fix:   "install the binary or fix the path — the server silently fails to start",
				})
			}
		}

		env, _ := entry["env"].(map[string]any)
		var envKeys []string
		for k := range env {
			envKeys = append(envKeys, k)
		}
		sort.Strings(envKeys)
		for _, k := range envKeys {
			v, _ := env[k].(string)
			if v == "" {
				continue
			}
			if kinds := redact.FindNames(v); len(kinds) > 0 {
				out = append(out, Finding{
					File: path, Scope: scope, Dimension: "mcp", Severity: SeverityCritical,
					Issue: fmt.Sprintf("server %q env %s holds an inline secret (%s)", name, k, strings.Join(kinds, ", ")),
					Fix:   "reference the secret via your shell env or a credential helper, and rotate it",
				})
			}
		}
	}
	return out
}

// commandMissing reports why an MCP server command won't resolve, or ""
// when it looks runnable. Env-var references can't be checked statically.
func commandMissing(command string, look func(string) (string, error)) string {
	if strings.ContainsAny(command, "$%") {
		return ""
	}
	if filepath.IsAbs(command) || strings.ContainsRune(command, os.PathSeparator) {
		if _, err := os.Stat(expandHome(command)); err != nil {
			return fmt.Sprintf("command %s does not exist", Tildify(command))
		}
		return ""
	}
	if _, err := look(command); err != nil {
		return fmt.Sprintf("command %q not found on PATH", command)
	}
	return ""
}
