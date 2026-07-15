package harness

import (
	"encoding/json"
	"fmt"
	"sort"
)

// analyzeKeybindings validates ~/.claude/keybindings.json and flags
// duplicate bindings: two entries in the bindings array claiming the
// same key in the same context (later entries silently win). The schema
// check is deliberately loose — unknown shapes just pass.
func analyzeKeybindings(path string, content []byte) (parses bool, parseErr string, out []Finding) {
	var cfg map[string]any
	if err := json.Unmarshal(content, &cfg); err != nil {
		return false, err.Error(), []Finding{{
			File: path, Scope: "user", Dimension: "keybindings", Severity: SeverityWarning,
			Issue: "not valid JSON — Claude Code will ignore your keybindings",
			Fix:   "repair the syntax (" + err.Error() + ")",
		}}
	}

	bindings, _ := cfg["bindings"].([]any)
	seen := map[string]int{}
	for _, b := range bindings {
		bm, _ := b.(map[string]any)
		key, _ := bm["key"].(string)
		if key == "" {
			continue
		}
		context, _ := bm["context"].(string)
		if context == "" {
			context, _ = bm["when"].(string)
		}
		seen[context+"\x00"+key]++
	}

	var dupes []string
	for combo, n := range seen {
		if n > 1 {
			dupes = append(dupes, combo)
		}
	}
	sort.Strings(dupes)
	for _, combo := range dupes {
		context, key := splitCombo(combo)
		where := ""
		if context != "" {
			where = fmt.Sprintf(" in context %q", context)
		}
		out = append(out, Finding{
			File: path, Scope: "user", Dimension: "keybindings", Severity: SeverityInfo,
			Issue: fmt.Sprintf("key %q is bound %d times%s — only the last binding wins", key, seen[combo], where),
			Fix:   "remove the earlier bindings",
		})
	}
	return true, "", out
}

func splitCombo(combo string) (context, key string) {
	for i := 0; i < len(combo); i++ {
		if combo[i] == '\x00' {
			return combo[:i], combo[i+1:]
		}
	}
	return "", combo
}
