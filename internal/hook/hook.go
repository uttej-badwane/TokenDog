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
	"grep":    "grep",
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
// Two output shapes depending on whether the rewritten command contains a
// chain operator:
//
//	(no chain)  TD_SESSION_ID=abc TD_TRANSCRIPT_PATH='/p' td git status
//	(chain)     export TD_SESSION_ID=abc TD_TRANSCRIPT_PATH='/p'; cd /a && td git status
//
// The leading-assignment form scopes vars to the first command only — when
// the rewrite produced a chain (cd /a && td git status), the `td` segment
// would never see them. The export form propagates across the chain. Each
// Bash tool call runs in its own bash subshell, so export's process-level
// scope dies with that invocation — no cross-command pollution.
func injectSessionEnv(cmd, sessionID, transcriptPath string) string {
	var assignments []string
	if isSafeSessionID(sessionID) {
		assignments = append(assignments, "TD_SESSION_ID="+sessionID)
	}
	if isSafeTranscriptPath(transcriptPath) {
		assignments = append(assignments, "TD_TRANSCRIPT_PATH='"+transcriptPath+"'")
	}
	if len(assignments) == 0 {
		return cmd
	}
	joined := strings.Join(assignments, " ")
	if hasChainOp(cmd) {
		return "export " + joined + "; " + cmd
	}
	return joined + " " + cmd
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
	return rewriteCommand(cmd, 0)
}

// maxRewriteDepth bounds the recursion through bash -c / chain-segment
// re-entry. Plausible nest is bash -c → chain → segment → bash -c, i.e. 4.
// Anything deeper is almost certainly malformed input — bail out unchanged.
const maxRewriteDepth = 4

func rewriteCommand(cmd string, depth int) string {
	if depth > maxRewriteDepth {
		return cmd
	}
	cmd = strings.TrimSpace(cmd)
	if strings.HasPrefix(cmd, "td ") || strings.HasPrefix(cmd, "tokendog ") {
		return cmd
	}

	// `bash -c "<inner>"` / `sh -c '<inner>'` / `zsh -c <inner>` — Claude
	// often wraps complex pipelines this way. Rewriting the outer `bash`
	// would be a no-op (we don't filter bash itself), so unwrap the inner
	// command, rewrite it, and re-quote. If the inner command is itself
	// unrewritable, the result is identical to the input.
	if rewritten, ok := rewriteShellC(cmd, depth); ok {
		return rewritten
	}

	// Chain operators: `cd /path && git status`, `git status; ls`. Bash
	// runs each segment as an independent command, so we rewrite each one
	// independently — the chain operators keep their original meaning at
	// exec time. splitChain bails out (returns 1 segment) on inputs we
	// can't safely split (backticks, $(...), heredocs, escaped operators).
	if segs, seps := splitChain(cmd); len(segs) > 1 {
		rewritten := make([]string, len(segs))
		changed := false
		for i, s := range segs {
			r := rewriteCommand(s, depth+1)
			rewritten[i] = r
			if r != s {
				changed = true
			}
		}
		if !changed {
			return cmd
		}
		return reassembleChain(rewritten, seps)
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

// reassembleChain stitches segments back together with their original
// separators. Always emits separator-with-spaces form (` && `, ` ; `, etc.)
// even if the original used different spacing — bash accepts either, and
// canonicalizing keeps the tests deterministic.
func reassembleChain(segs, seps []string) string {
	var b strings.Builder
	for i, s := range segs {
		b.WriteString(s)
		if i < len(seps) {
			b.WriteString(" ")
			b.WriteString(seps[i])
			b.WriteString(" ")
		}
	}
	return b.String()
}

// splitChain splits cmd on top-level && / || / ; outside quoted regions.
// Returns segments and the operators between them. When the input contains
// command substitution (`$(...)` / backticks), heredocs (`<<`), or
// backslash-escaped chain operators, returns a single-segment slice — we
// can't reliably parse those without a real shell parser, and a wrong
// split would produce malformed shell. Safety > coverage.
func splitChain(cmd string) (segments, separators []string) {
	if strings.TrimSpace(cmd) == "" {
		return []string{cmd}, nil
	}
	if !safeToSplit(cmd) {
		return []string{cmd}, nil
	}

	var current strings.Builder
	var inSingle, inDouble bool
	flush := func(sep string) {
		segments = append(segments, strings.TrimSpace(current.String()))
		separators = append(separators, sep)
		current.Reset()
	}

	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		if inSingle {
			current.WriteByte(c)
			if c == '\'' {
				inSingle = false
			}
			continue
		}
		if inDouble {
			current.WriteByte(c)
			if c == '\\' && i+1 < len(cmd) {
				current.WriteByte(cmd[i+1])
				i++
				continue
			}
			if c == '"' {
				inDouble = false
			}
			continue
		}
		switch c {
		case '\'':
			inSingle = true
			current.WriteByte(c)
		case '"':
			inDouble = true
			current.WriteByte(c)
		case '\\':
			// Escaped char outside quotes: copy both bytes verbatim so
			// `\&\&` doesn't get treated as a chain op.
			current.WriteByte(c)
			if i+1 < len(cmd) {
				current.WriteByte(cmd[i+1])
				i++
			}
		case '&':
			if i+1 < len(cmd) && cmd[i+1] == '&' {
				flush("&&")
				i++
				continue
			}
			current.WriteByte(c) // single & = background, leave alone
		case '|':
			if i+1 < len(cmd) && cmd[i+1] == '|' {
				flush("||")
				i++
				continue
			}
			current.WriteByte(c) // single | = pipe, leave alone
		case ';':
			flush(";")
		default:
			current.WriteByte(c)
		}
	}
	if current.Len() > 0 || len(segments) > 0 {
		segments = append(segments, strings.TrimSpace(current.String()))
	}
	if len(segments) == 0 {
		return []string{cmd}, nil
	}
	return segments, separators
}

// safeToSplit returns false for inputs containing constructs we can't
// reliably handle: command substitution, backticks, and heredocs. We
// scan only outside of quoted regions because `echo "$(date)"` is fine —
// the substitution is inside double quotes and we're not splitting there
// anyway. Outside quotes, any of these constructs means we bail.
func safeToSplit(cmd string) bool {
	var inSingle, inDouble bool
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		if inSingle {
			if c == '\'' {
				inSingle = false
			}
			continue
		}
		if inDouble {
			if c == '\\' && i+1 < len(cmd) {
				i++
				continue
			}
			if c == '"' {
				inDouble = false
			}
			continue
		}
		switch c {
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		case '\\':
			if i+1 < len(cmd) {
				i++
			}
		case '`':
			return false
		case '$':
			if i+1 < len(cmd) && cmd[i+1] == '(' {
				return false
			}
		case '<':
			if i+1 < len(cmd) && cmd[i+1] == '<' {
				return false
			}
		}
	}
	return true
}

// hasChainOp reports whether splitChain would produce more than one segment
// for cmd. Used by injectSessionEnv to pick between leading-assignment and
// export forms.
func hasChainOp(cmd string) bool {
	segs, _ := splitChain(cmd)
	return len(segs) > 1
}

// rewriteShellC detects `<shell> -c <inner>` and rewrites the inner command,
// preserving the original quoting style. Returns (rewritten, true) on a
// successful rewrite, (cmd, false) otherwise. We handle the three common
// shells (bash/sh/zsh) that Claude Code might emit. depth is forwarded to
// the recursive rewrite so the global recursion guard sees nested chains.
func rewriteShellC(cmd string, depth int) (string, bool) {
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

	rewritten := rewriteCommand(inner, depth+1)
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

// ParseBinary extracts the underlying binary and its remaining args from a
// Claude-emitted command string. Mirrors RewriteCommand's parsing rules
// (env-var prefix skipping, path-prefix stripping, bash -c unwrapping,
// Supported lookup) but returns the parsed pieces instead of a rewritten
// string. Used by td replay to classify historical transcripts so the same
// command ends up in the same filter live and on replay.
//
// Returns (binary, args, true) when a Supported binary is found,
// ("", nil, false) otherwise. binary is the canonical name (e.g. "git"),
// not the path-prefixed form Claude may have emitted.
func ParseBinary(cmd string) (string, []string, bool) {
	cmd = strings.TrimSpace(cmd)

	// Already wrapped with td? Strip the prefix and parse the rest. Useful
	// for replaying records that already have td hook injection (post-v0.5.0).
	if strings.HasPrefix(cmd, "td ") {
		cmd = strings.TrimPrefix(cmd, "td ")
	} else if strings.HasPrefix(cmd, "tokendog ") {
		cmd = strings.TrimPrefix(cmd, "tokendog ")
	}

	if inner, ok := unwrapShellC(cmd); ok {
		return ParseBinary(inner)
	}

	// Chain-aware classification: if the command is a chain, return the
	// single Supported segment if exactly one exists, else give up. Two
	// supported segments (e.g. `git status; ls`) means the tool_result
	// in a transcript holds both outputs concatenated — applying one
	// tool's filter to the combined blob would mis-classify the other.
	// Conservative skip; replay treats it as unhandled.
	if segs, _ := splitChain(cmd); len(segs) > 1 {
		var matchBin string
		var matchArgs []string
		matches := 0
		for _, s := range segs {
			if bin, args, ok := ParseBinary(s); ok {
				matchBin, matchArgs = bin, args
				matches++
			}
		}
		if matches == 1 {
			return matchBin, matchArgs, true
		}
		return "", nil, false
	}

	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return "", nil, false
	}
	binIdx := 0
	for binIdx < len(parts) && IsEnvAssignment(parts[binIdx]) {
		binIdx++
	}
	if binIdx >= len(parts) {
		return "", nil, false
	}
	bin := parts[binIdx]
	if i := strings.LastIndex(bin, "/"); i >= 0 {
		bin = bin[i+1:]
	}
	if _, ok := Supported[bin]; !ok {
		return "", nil, false
	}
	return bin, parts[binIdx+1:], true
}

// unwrapShellC returns the inner command string from `bash -c "..."` style
// wrappers without rewriting it. Counterpart to rewriteShellC for callers
// that just want to unwrap and process the inner command themselves.
func unwrapShellC(cmd string) (string, bool) {
	for _, shell := range []string{"bash", "sh", "zsh"} {
		for _, flag := range []string{"-lc", "-ic", "-c"} {
			prefix := shell + " " + flag + " "
			if !strings.HasPrefix(cmd, prefix) {
				continue
			}
			rest := strings.TrimPrefix(cmd, prefix)
			inner, _, _, ok := unquoteShellArg(rest)
			if ok {
				return inner, true
			}
		}
	}
	return "", false
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
