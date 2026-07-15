package harness

import (
	"fmt"
	"sort"
)

// analyzeCrossScope compares scalar (string/number/bool) top-level keys
// set in both the user and project settings.json. Different values mean
// the project silently overrides the user — worth knowing. Identical
// values mean the project entry is dead weight restating the user
// config. Maps (permissions, hooks, env) merge across scopes by design
// and are skipped.
func analyzeCrossScope(user, proj map[string]any, userPath, projPath string) []Finding {
	if user == nil || proj == nil {
		return nil
	}
	var keys []string
	for k, v := range proj {
		if _, inUser := user[k]; !inUser || !isScalar(v) || !isScalar(user[k]) {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var out []Finding
	for _, k := range keys {
		if user[k] == proj[k] {
			out = append(out, Finding{
				File: projPath, Scope: "project", Dimension: "cross-scope", Severity: SeverityInfo,
				Issue: fmt.Sprintf("%q restates the identical value from %s", k, Tildify(userPath)),
				Fix:   "remove the project entry — the user setting already applies",
			})
		} else {
			out = append(out, Finding{
				File: projPath, Scope: "project", Dimension: "cross-scope", Severity: SeverityInfo,
				Issue: fmt.Sprintf("%q is set in both scopes with different values — project (%v) overrides user (%v)", k, proj[k], user[k]),
				Fix:   "keep whichever is intended and remove the other to avoid surprises",
			})
		}
	}
	return out
}

func isScalar(v any) bool {
	switch v.(type) {
	case string, float64, bool, nil:
		return true
	}
	return false
}
