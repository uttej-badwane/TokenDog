package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

// unsafeHookPatterns are shell idioms that execute content nobody
// reviewed. Deliberately few and unambiguous — a hook audit that cries
// wolf gets ignored.
var unsafeHookPatterns = []struct {
	re    *regexp.Regexp
	issue string
}{
	{regexp.MustCompile(`curl[^|;&]*\|\s*(?:sudo\s+)?(?:ba|z|da)?sh\b`), "pipes curl output straight into a shell"},
	{regexp.MustCompile(`wget[^|;&]*\|\s*(?:sudo\s+)?(?:ba|z|da)?sh\b`), "pipes wget output straight into a shell"},
	{regexp.MustCompile(`\beval\s+["']?\$\(`), "evals the output of a command substitution"},
}

// interpreters whose first argument is the actual script to check.
var hookInterpreters = map[string]bool{
	"sh": true, "bash": true, "zsh": true, "dash": true,
	"python": true, "python3": true, "node": true, "bun": true, "deno": true,
}

// shellMeta reports whether the command uses shell syntax beyond a
// simple invocation — in that case we skip existence checks (we can't
// statically resolve pipelines, conditionals, or env expansion).
func shellMeta(cmd string) bool {
	return strings.ContainsAny(cmd, "|;&<>(){}")
}

// analyzeHooks walks the hooks tree in one settings file:
//
//	{"PreToolUse": [{"matcher": "...", "hooks": [{"type": "command", "command": "..."}]}]}
//
// and checks each command for unsafe patterns, a script that doesn't
// exist, and (unix) a script missing its exec bit — the latter is
// auto-fixable via `td harness apply`.
func analyzeHooks(path, scope string, cfg map[string]any, look func(string) (string, error)) []Finding {
	hooks, _ := cfg["hooks"].(map[string]any)
	if hooks == nil {
		return nil
	}
	events := make([]string, 0, len(hooks))
	for event := range hooks {
		events = append(events, event)
	}
	sort.Strings(events)

	var out []Finding
	for _, event := range events {
		groups, _ := hooks[event].([]any)
		for _, g := range groups {
			gm, _ := g.(map[string]any)
			entries, _ := gm["hooks"].([]any)
			for _, e := range entries {
				em, _ := e.(map[string]any)
				cmd, _ := em["command"].(string)
				if cmd == "" {
					continue
				}
				out = append(out, checkHookCommand(path, scope, event, cmd, look)...)
			}
		}
	}
	return out
}

func checkHookCommand(path, scope, event, cmd string, look func(string) (string, error)) []Finding {
	var out []Finding
	for _, p := range unsafeHookPatterns {
		if p.re.MatchString(cmd) {
			out = append(out, Finding{
				File: path, Scope: scope, Dimension: "hooks", Severity: SeverityCritical,
				Issue: fmt.Sprintf("%s hook %s: %q", event, p.issue, truncate(cmd, 60)),
				Fix:   "download to a file, review it, then execute — hooks run unsandboxed with your credentials",
			})
		}
	}

	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return out
	}
	first := fields[0]

	// Resolve the thing being executed: either the first token, or the
	// first non-flag argument after a known interpreter.
	script := first
	viaInterpreter := false
	if hookInterpreters[filepath.Base(first)] {
		viaInterpreter = true
		script = ""
		for _, f := range fields[1:] {
			if !strings.HasPrefix(f, "-") {
				script = f
				break
			}
		}
	}
	// Unresolvable statically: env expansion ($CLAUDE_PROJECT_DIR/…),
	// or complex shell where the first token isn't the real command.
	if script == "" || strings.ContainsAny(script, "$`'\"") {
		return out
	}

	if strings.ContainsRune(script, os.PathSeparator) || strings.HasPrefix(script, "~") {
		full := expandHome(script)
		info, err := os.Stat(full)
		if err != nil {
			out = append(out, Finding{
				File: path, Scope: scope, Dimension: "hooks", Severity: SeverityWarning,
				Issue: fmt.Sprintf("%s hook script %s does not exist", event, Tildify(full)),
				Fix:   "fix the path or remove the hook — it fails silently on every trigger",
			})
			return out
		}
		if !viaInterpreter && runtime.GOOS != "windows" && info.Mode().IsRegular() && info.Mode().Perm()&0111 == 0 {
			out = append(out, Finding{
				File: path, Scope: scope, Dimension: "hooks", Severity: SeverityWarning,
				Issue:       fmt.Sprintf("%s hook script %s is not executable", event, Tildify(full)),
				Fix:         "chmod +x it",
				AutoFixable: true,
				FixID:       fixIDHookExec(full),
			})
		}
		return out
	}

	// Bare command name: only meaningful to check when the command is a
	// simple invocation (no pipeline/conditional around it).
	if !viaInterpreter && !shellMeta(cmd) {
		if _, err := look(first); err != nil {
			out = append(out, Finding{
				File: path, Scope: scope, Dimension: "hooks", Severity: SeverityWarning,
				Issue: fmt.Sprintf("%s hook command %q not found on PATH", event, first),
				Fix:   "install it or use an absolute path — the hook fails silently on every trigger",
			})
		}
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
