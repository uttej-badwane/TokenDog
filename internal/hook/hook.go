package hook

import "strings"

type ClaudeHookInput struct {
	SessionID      string         `json:"session_id"`
	TranscriptPath string         `json:"transcript_path"`
	ToolName       string         `json:"tool_name"`
	ToolInput      map[string]any `json:"tool_input"`
}

type ClaudeHookOutput struct {
	ToolInput map[string]any `json:"tool_input,omitempty"`
}

// supported maps the leading binary name to its tokendog subcommand
var supported = map[string]string{
	"git":    "git",
	"ls":     "ls",
	"find":   "find",
	"docker": "docker",
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
	return &ClaudeHookOutput{ToolInput: newInput}
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
	bin := parts[0]
	if idx := strings.LastIndex(bin, "/"); idx >= 0 {
		bin = bin[idx+1:]
	}
	sub, ok := supported[bin]
	if !ok {
		return cmd
	}
	rest := strings.Join(parts[1:], " ")
	if rest == "" {
		return "td " + sub
	}
	return "td " + sub + " " + rest
}
