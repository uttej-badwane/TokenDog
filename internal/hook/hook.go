package hook

import "strings"

type ClaudeHookInput struct {
	SessionID      string         `json:"session_id"`
	TranscriptPath string         `json:"transcript_path"`
	ToolName       string         `json:"tool_name"`
	ToolInput      map[string]any `json:"tool_input"`
}

type ClaudeHookOutput struct {
	HookSpecificOutput *HookSpecificOutput `json:"hookSpecificOutput,omitempty"`
}

type HookSpecificOutput struct {
	HookEventName string         `json:"hookEventName"`
	UpdatedInput  map[string]any `json:"updatedInput"`
}

// Supported maps the leading binary name to its tokendog subcommand. It is
// the single source of truth for "td knows how to filter this" — `td
// discover` reads it to classify history rows, and the hook reads it to
// decide whether to rewrite. Treat as read-only.
var Supported = map[string]string{
	"git":     "git",
	"ls":      "ls",
	"find":    "find",
	"docker":  "docker",
	"jq":      "jq",
	"curl":    "curl",
	"kubectl": "kubectl",
	"gh":      "gh",
	"pytest":  "pytest",
	"jest":    "jest",
	"vitest":  "vitest",
	"go":      "go",
	"cargo":   "cargo",
	"npm":     "npm",
	"pnpm":    "pnpm",
	"yarn":    "yarn",
	"pip":     "pip",
	"aws":     "aws",
	"gcloud":  "gcloud",
	"az":      "az",
	"make":    "make",
}

func ProcessClaude(input ClaudeHookInput) *ClaudeHookOutput {
	if input.ToolName != "Bash" {
		return nil
	}
	cmd, ok := input.ToolInput["command"].(string)
	if !ok || cmd == "" {
		return nil
	}
	rewritten := RewriteCommand(cmd)
	if rewritten == cmd {
		return nil
	}
	rewritten = injectSessionEnv(rewritten, input.SessionID, input.TranscriptPath)

	newInput := make(map[string]any, len(input.ToolInput))
	for k, v := range input.ToolInput {
		newInput[k] = v
	}
	newInput["command"] = rewritten
	return &ClaudeHookOutput{
		HookSpecificOutput: &HookSpecificOutput{
			HookEventName: "PreToolUse",
			UpdatedInput:  newInput,
		},
	}
}

// injectSessionEnv prepends TD_SESSION_ID + TD_TRANSCRIPT_PATH env-var
// assignments to the rewritten command so each `td <tool>` exec inherits the
// session context. Both values are validated for shell-safety; if either
// would require non-trivial escaping, that field is dropped rather than
// risking command injection. Empty values are skipped silently.
//
// The rewritten form looks like:
//
//	TD_SESSION_ID=abc123 TD_TRANSCRIPT_PATH='/path/file.jsonl' td git status
//
// IsEnvAssignment + the existing env-prefix-skipping logic in RewriteCommand
// already handle this shape, so subsequent rewrites (e.g. through bash -c
// recursion) won't be confused.
func injectSessionEnv(cmd, sessionID, transcriptPath string) string {
	prefix := ""
	if isSafeSessionID(sessionID) {
		prefix += "TD_SESSION_ID=" + sessionID + " "
	}
	if isSafeTranscriptPath(transcriptPath) {
		prefix += "TD_TRANSCRIPT_PATH='" + transcriptPath + "' "
	}
	if prefix == "" {
		return cmd
	}
	return prefix + cmd
}

// isSafeSessionID accepts UUIDs and similar opaque IDs: alphanumerics,
// hyphens, underscores. Anything else (newlines, quotes, semicolons) means
// we drop the env var rather than risk producing a malformed command.
func isSafeSessionID(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		ok := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_'
		if !ok {
			return false
		}
	}
	return true
}

// isSafeTranscriptPath rejects paths containing single quotes (which would
// break our single-quote wrapping), newlines, or backslashes. Real Claude
// transcript paths under ~/.claude/projects/ never contain any of these.
func isSafeTranscriptPath(s string) bool {
	if s == "" || len(s) > 4096 {
		return false
	}
	for _, r := range s {
		if r == '\'' || r == '\n' || r == '\r' || r == '\\' || r == 0 {
			return false
		}
	}
	return true
}

func RewriteCommand(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if strings.HasPrefix(cmd, "td ") || strings.HasPrefix(cmd, "tokendog ") {
		return cmd
	}

	// `bash -c "<inner>"` / `sh -c '<inner>'` / `zsh -c <inner>` — Claude
	// often wraps complex pipelines this way. Rewriting the outer `bash`
	// would be a no-op (we don't filter bash itself), so unwrap the inner
	// command, rewrite it, and re-quote. If the inner command is itself
	// unrewritable, the result is identical to the input.
	if rewritten, ok := rewriteShellC(cmd); ok {
		return rewritten
	}

	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return cmd
	}

	// Skip leading env-var assignments (e.g. `AWS_PROFILE=foo aws ec2 ...`)
	// so the hook still recognizes the underlying binary.
	binIdx := 0
	for binIdx < len(parts) && IsEnvAssignment(parts[binIdx]) {
		binIdx++
	}
	if binIdx >= len(parts) {
		return cmd
	}

	bin := parts[binIdx]
	if idx := strings.LastIndex(bin, "/"); idx >= 0 {
		bin = bin[idx+1:]
	}
	sub, ok := Supported[bin]
	if !ok {
		return cmd
	}

	prefix := strings.Join(parts[:binIdx], " ")
	rest := strings.Join(parts[binIdx+1:], " ")

	out := "td " + sub
	if prefix != "" {
		out = prefix + " " + out
	}
	if rest != "" {
		out = out + " " + rest
	}
	return out
}

// rewriteShellC detects `<shell> -c <inner>` and rewrites the inner command,
// preserving the original quoting style. Returns (rewritten, true) on a
// successful rewrite, (cmd, false) otherwise. We handle the three common
// shells (bash/sh/zsh) that Claude Code might emit.
func rewriteShellC(cmd string) (string, bool) {
	var shell string
	for _, s := range []string{"bash", "sh", "zsh"} {
		if strings.HasPrefix(cmd, s+" -c ") || strings.HasPrefix(cmd, s+" -lc ") || strings.HasPrefix(cmd, s+" -ic ") {
			shell = s
			break
		}
	}
	if shell == "" {
		return cmd, false
	}

	// Find the start of the inner string — skip past the shell binary, all
	// short flags ending in `c`, and any whitespace.
	rest := strings.TrimSpace(strings.TrimPrefix(cmd, shell))
	flag := ""
	for _, candidate := range []string{"-lc", "-ic", "-c"} {
		if strings.HasPrefix(rest, candidate+" ") {
			flag = candidate
			rest = strings.TrimSpace(strings.TrimPrefix(rest, candidate))
			break
		}
	}
	if flag == "" {
		return cmd, false
	}

	inner, quote, trailing, ok := unquoteShellArg(rest)
	if !ok {
		return cmd, false
	}

	rewritten := RewriteCommand(inner)
	if rewritten == inner {
		return cmd, false
	}

	// Re-quote using the original style. If the rewritten content contains
	// the original quote char (rare — only happens with embedded quotes),
	// fall through to no-rewrite to avoid producing a syntactically broken
	// command. The user gets the unrewritten passthrough; correctness wins
	// over savings.
	if quote != 0 && strings.ContainsRune(rewritten, quote) {
		return cmd, false
	}

	var quoted string
	switch quote {
	case '\'':
		quoted = "'" + rewritten + "'"
	case '"':
		quoted = "\"" + rewritten + "\""
	default:
		// No quotes in original — preserve that, but only if the rewritten
		// command has no spaces that would change argv. It almost always
		// does (`td git status`), so quote it defensively with single quotes.
		quoted = "'" + rewritten + "'"
	}

	out := shell + " " + flag + " " + quoted
	if trailing != "" {
		out += " " + trailing
	}
	return out, true
}

// unquoteShellArg parses the leading shell-quoted argument from s and returns
// (content, quote-rune, trailing, ok). The quote-rune is 0 if the argument
// was unquoted. trailing is anything after the parsed argument (extra args
// to the shell, e.g. `bash -c 'cmd' arg0 arg1`). Handles `'...'` and `"..."`
// with backslash escapes for double quotes only — single quotes are literal
// in POSIX shells. Returns ok=false on malformed/unterminated quoting.
func unquoteShellArg(s string) (string, rune, string, bool) {
	if s == "" {
		return "", 0, "", false
	}
	first := rune(s[0])
	if first != '\'' && first != '"' {
		// Unquoted: take the first whitespace-delimited token.
		idx := strings.IndexAny(s, " \t")
		if idx < 0 {
			return s, 0, "", true
		}
		return s[:idx], 0, strings.TrimSpace(s[idx:]), true
	}

	var b strings.Builder
	i := 1
	for i < len(s) {
		c := s[i]
		if rune(c) == first {
			// End of quoted region.
			trailing := strings.TrimSpace(s[i+1:])
			return b.String(), first, trailing, true
		}
		if first == '"' && c == '\\' && i+1 < len(s) {
			next := s[i+1]
			if next == '"' || next == '\\' || next == '$' || next == '`' {
				b.WriteByte(next)
				i += 2
				continue
			}
		}
		b.WriteByte(c)
		i++
	}
	// Unterminated quote.
	return "", 0, "", false
}

// IsEnvAssignment reports whether s is a shell env-var assignment of the
// form NAME=VALUE where NAME starts with a letter or underscore and contains
// only letters, digits, and underscores. Quoted values with embedded spaces
// are not handled (Fields would have split them already), but the common
// `AWS_PROFILE=foo`, `DEBUG=1`, `PATH=/x:/y` shapes all match.
func IsEnvAssignment(s string) bool {
	eq := strings.IndexByte(s, '=')
	if eq <= 0 {
		return false
	}
	name := s[:eq]
	for i := 0; i < len(name); i++ {
		c := name[i]
		isAlpha := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		isDigit := c >= '0' && c <= '9'
		if i == 0 && !(isAlpha || c == '_') {
			return false
		}
		if !(isAlpha || isDigit || c == '_') {
			return false
		}
	}
	return true
}
