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

// supported maps the leading binary name to its tokendog subcommand
var supported = map[string]string{
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

func RewriteCommand(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if strings.HasPrefix(cmd, "td ") || strings.HasPrefix(cmd, "tokendog ") {
		return cmd
	}
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return cmd
	}

	// Skip leading env-var assignments (e.g. `AWS_PROFILE=foo aws ec2 ...`)
	// so the hook still recognizes the underlying binary.
	binIdx := 0
	for binIdx < len(parts) && isEnvAssignment(parts[binIdx]) {
		binIdx++
	}
	if binIdx >= len(parts) {
		return cmd
	}

	bin := parts[binIdx]
	if idx := strings.LastIndex(bin, "/"); idx >= 0 {
		bin = bin[idx+1:]
	}
	sub, ok := supported[bin]
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

// isEnvAssignment reports whether s is a shell env-var assignment of the
// form NAME=VALUE where NAME starts with a letter or underscore and contains
// only letters, digits, and underscores. Quoted values with embedded spaces
// are not handled (Fields would have split them already), but the common
// `AWS_PROFILE=foo`, `DEBUG=1`, `PATH=/x:/y` shapes all match.
func isEnvAssignment(s string) bool {
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
