package harness

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
)

// parseFrontmatter extracts the leading YAML block between --- fences.
// Returns the top-level scalar fields (nested structures are skipped —
// enough for name/description/tools checks without a YAML dependency),
// whether a block was present at all, and an error when a block was
// opened but never closed.
func parseFrontmatter(content []byte) (fields map[string]string, present bool, err error) {
	sc := bufio.NewScanner(bytes.NewReader(content))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	if !sc.Scan() || strings.TrimRight(sc.Text(), "\r") != "---" {
		return nil, false, nil
	}
	fields = map[string]string{}
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if line == "---" {
			return fields, true, nil
		}
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		if key, value, ok := strings.Cut(line, ":"); ok {
			fields[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
		}
	}
	return fields, true, fmt.Errorf("frontmatter opened with --- but never closed")
}

// analyzeAgentFile checks one agents/*.md definition: frontmatter must
// parse and carry name + description (Claude Code won't route to the
// agent otherwise); a missing tools list is only informational.
func analyzeAgentFile(path, scope string, content []byte) (parses bool, parseErr string, out []Finding) {
	fields, present, err := parseFrontmatter(content)
	if !present {
		return false, "no frontmatter", []Finding{{
			File: path, Scope: scope, Dimension: "agents", Severity: SeverityWarning,
			Issue: "agent file has no frontmatter — Claude Code won't load it",
			Fix:   "add a --- block with at least name and description",
		}}
	}
	if err != nil {
		return false, err.Error(), []Finding{{
			File: path, Scope: scope, Dimension: "agents", Severity: SeverityWarning,
			Issue: "frontmatter block is malformed (" + err.Error() + ")",
			Fix:   "close the --- fence",
		}}
	}
	for _, key := range []string{"name", "description"} {
		if fields[key] == "" {
			out = append(out, Finding{
				File: path, Scope: scope, Dimension: "agents", Severity: SeverityWarning,
				Issue: fmt.Sprintf("frontmatter is missing %q — the agent can't be selected without it", key),
				Fix:   "add the field; make description specific about when to trigger this agent",
			})
		}
	}
	if desc := fields["description"]; desc != "" && len(desc) < 20 {
		out = append(out, Finding{
			File: path, Scope: scope, Dimension: "agents", Severity: SeverityInfo,
			Issue: fmt.Sprintf("description %q is too generic to trigger reliably", desc),
			Fix:   "describe the specific tasks and phrasings that should route to this agent",
		})
	}
	if _, ok := fields["tools"]; !ok {
		out = append(out, Finding{
			File: path, Scope: scope, Dimension: "agents", Severity: SeverityInfo,
			Issue: "no tools list — the agent inherits every tool",
			Fix:   "add `tools:` with just what it needs (least privilege)",
		})
	}
	return true, "", out
}

// analyzeSkillFile checks one skills/<name>/SKILL.md: frontmatter with
// name + description is what makes the skill discoverable.
func analyzeSkillFile(path, scope string, content []byte) (parses bool, parseErr string, out []Finding) {
	fields, present, err := parseFrontmatter(content)
	if !present {
		return false, "no frontmatter", []Finding{{
			File: path, Scope: scope, Dimension: "skills", Severity: SeverityWarning,
			Issue: "SKILL.md has no frontmatter — the skill won't be discovered",
			Fix:   "add a --- block with name and description",
		}}
	}
	if err != nil {
		return false, err.Error(), []Finding{{
			File: path, Scope: scope, Dimension: "skills", Severity: SeverityWarning,
			Issue: "frontmatter block is malformed (" + err.Error() + ")",
			Fix:   "close the --- fence",
		}}
	}
	for _, key := range []string{"name", "description"} {
		if fields[key] == "" {
			out = append(out, Finding{
				File: path, Scope: scope, Dimension: "skills", Severity: SeverityWarning,
				Issue: fmt.Sprintf("frontmatter is missing %q", key),
				Fix:   "add the field — description is what tells Claude when to invoke the skill",
			})
		}
	}
	return true, "", out
}
