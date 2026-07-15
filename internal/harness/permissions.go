package harness

import (
	"fmt"
	"strings"
)

// ruleSpec is a parsed permission rule: "Bash(git *)" → {tool: "Bash",
// pattern: "git *"}; a bare "Bash" (no parens) matches everything for
// that tool.
type ruleSpec struct {
	tool    string
	pattern string
	bare    bool
}

func parseRule(r string) ruleSpec {
	open := strings.IndexByte(r, '(')
	if open < 0 || !strings.HasSuffix(r, ")") {
		return ruleSpec{tool: r, bare: true}
	}
	return ruleSpec{tool: r[:open], pattern: r[open+1 : len(r)-1]}
}

// ruleCovers reports whether rule a matches every command rule b
// matches (a strictly-or-equally broader rule). Prefix-glob semantics:
// a trailing * matches any suffix, which handles both the modern
// "git *" and the older "git:*" styles.
func ruleCovers(a, b ruleSpec) bool {
	if a.tool != b.tool {
		return false
	}
	if a.bare || a.pattern == "*" {
		return true
	}
	if b.bare {
		return false
	}
	if strings.HasSuffix(a.pattern, "*") {
		return strings.HasPrefix(b.pattern, strings.TrimSuffix(a.pattern, "*"))
	}
	return false
}

// broadRule explains why an allow rule is dangerously broad, or "" when
// it isn't. Only allow rules are checked — a broad deny/ask is safe.
func broadRule(r ruleSpec) string {
	if r.tool == "*" || (r.bare && r.tool == "") {
		return "allows every tool without prompting"
	}
	if r.tool == "Bash" && (r.bare || r.pattern == "*" || r.pattern == "*:*" || r.pattern == ":*") {
		return "allows any shell command without prompting"
	}
	if strings.HasPrefix(r.tool, "mcp__") && r.bare && strings.Count(r.tool, "__") == 1 {
		return "allows every tool on that MCP server without prompting"
	}
	return ""
}

// analyzePermissions inspects permissions.allow/deny/ask in one settings
// file: overly broad allows (warning), exact duplicates (info,
// auto-fixable), and rules shadowed by a broader rule in the same list
// (info).
func analyzePermissions(path, scope string, cfg map[string]any) []Finding {
	perms, _ := cfg["permissions"].(map[string]any)
	if perms == nil {
		return nil
	}
	var out []Finding
	for _, list := range []string{"allow", "deny", "ask"} {
		raw, _ := perms[list].([]any)
		if len(raw) == 0 {
			continue
		}
		rules := make([]string, 0, len(raw))
		for _, v := range raw {
			if s, ok := v.(string); ok {
				rules = append(rules, s)
			}
		}

		// Exact duplicates — the one thing `td harness apply` fixes here.
		seen := map[string]bool{}
		reported := map[string]bool{}
		for _, r := range rules {
			if seen[r] && !reported[r] {
				reported[r] = true
				out = append(out, Finding{
					File: path, Scope: scope, Dimension: "permissions", Severity: SeverityInfo,
					Issue:       fmt.Sprintf("rule %q appears more than once in permissions.%s", r, list),
					Fix:         "remove the duplicate entries",
					AutoFixable: true,
					FixID:       fixIDDupPerm(path, list, r),
				})
			}
			seen[r] = true
		}

		specs := make([]ruleSpec, len(rules))
		for i, r := range rules {
			specs[i] = parseRule(r)
		}

		if list == "allow" {
			for i, spec := range specs {
				if why := broadRule(spec); why != "" {
					out = append(out, Finding{
						File: path, Scope: scope, Dimension: "permissions", Severity: SeverityWarning,
						Issue: fmt.Sprintf("allow rule %q %s", rules[i], why),
						Fix:   "narrow it to the specific commands you actually run (e.g. Bash(git *))",
					})
				}
			}
		}

		// Shadowed rules: covered by a distinct broader rule in the same
		// list. Report each shadowed rule once.
		for i, narrow := range specs {
			for j, broad := range specs {
				if i == j || rules[i] == rules[j] {
					continue
				}
				if ruleCovers(broad, narrow) {
					out = append(out, Finding{
						File: path, Scope: scope, Dimension: "permissions", Severity: SeverityInfo,
						Issue: fmt.Sprintf("rule %q in permissions.%s is already covered by %q", rules[i], list, rules[j]),
						Fix:   "remove the narrower rule",
					})
					break
				}
			}
		}
	}
	return out
}
