package hook

import (
	"encoding/json"
	"testing"
)

// TestProcessClaudeSchema is the regression test for v0.4.0's silent-rewrite
// bug. The hook MUST emit hookSpecificOutput.{hookEventName,updatedInput};
// any other shape causes Claude Code to ignore the rewrite without warning.
func TestProcessClaudeSchema(t *testing.T) {
	in := ClaudeHookInput{
		ToolName:  "Bash",
		ToolInput: map[string]any{"command": "ls /tmp"},
	}
	out := ProcessClaude(in)
	if out == nil {
		t.Fatal("ProcessClaude returned nil for a rewritable command")
	}
	if out.HookSpecificOutput == nil {
		t.Fatal("HookSpecificOutput is nil")
	}
	if out.HookSpecificOutput.HookEventName != "PreToolUse" {
		t.Errorf("HookEventName = %q, want %q", out.HookSpecificOutput.HookEventName, "PreToolUse")
	}
	cmd, ok := out.HookSpecificOutput.UpdatedInput["command"].(string)
	if !ok {
		t.Fatal("UpdatedInput.command missing or not a string")
	}
	if cmd != "td ls /tmp" {
		t.Errorf("rewritten command = %q, want %q", cmd, "td ls /tmp")
	}

	// Marshalled shape must match Claude Code's expected JSON exactly.
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	want := `{"hookSpecificOutput":{"hookEventName":"PreToolUse","updatedInput":{"command":"td ls /tmp"}}}`
	if got != want {
		t.Errorf("JSON = %s\nwant %s", got, want)
	}
}

func TestProcessClaudeNonBash(t *testing.T) {
	in := ClaudeHookInput{
		ToolName:  "Read",
		ToolInput: map[string]any{"file_path": "/tmp/x"},
	}
	if out := ProcessClaude(in); out != nil {
		t.Errorf("expected nil for non-Bash tool, got %+v", out)
	}
}

func TestProcessClaudeEmptyCommand(t *testing.T) {
	in := ClaudeHookInput{
		ToolName:  "Bash",
		ToolInput: map[string]any{"command": ""},
	}
	if out := ProcessClaude(in); out != nil {
		t.Errorf("expected nil for empty command, got %+v", out)
	}
}

func TestProcessClaudeNoRewrite(t *testing.T) {
	// Already prefixed with td — no double-wrapping
	in := ClaudeHookInput{
		ToolName:  "Bash",
		ToolInput: map[string]any{"command": "td git status"},
	}
	if out := ProcessClaude(in); out != nil {
		t.Errorf("expected nil when input already starts with td, got %+v", out)
	}

	// Unsupported binary
	in = ClaudeHookInput{
		ToolName:  "Bash",
		ToolInput: map[string]any{"command": "echo hello"},
	}
	if out := ProcessClaude(in); out != nil {
		t.Errorf("expected nil for unsupported binary, got %+v", out)
	}
}

func TestRewriteCommand(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain ls", "ls /tmp", "td ls /tmp"},
		{"git with subcmd", "git status", "td git status"},
		{"already td", "td git status", "td git status"},
		{"already tokendog", "tokendog git status", "tokendog git status"},
		{"unsupported binary", "echo hello", "echo hello"},
		{"empty", "", ""},
		{"path-prefixed binary", "/usr/local/bin/git status", "td git status"},
		{"single env-var prefix", "AWS_PROFILE=foo aws ec2 describe-instances", "AWS_PROFILE=foo td aws ec2 describe-instances"},
		{"multiple env-vars", "DEBUG=1 PATH=/x:/y npm test", "DEBUG=1 PATH=/x:/y td npm test"},
		{"env-var only — no command", "FOO=bar", "FOO=bar"},
		{"env-var unsupported binary", "FOO=bar grep pattern", "FOO=bar grep pattern"},
		{"binary with no args", "git", "td git"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RewriteCommand(tc.in); got != tc.want {
				t.Errorf("RewriteCommand(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestIsEnvAssignment(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"AWS_PROFILE=foo", true},
		{"DEBUG=1", true},
		{"PATH=/x:/y", true},
		{"_X=1", true},
		{"a=", true}, // empty value is allowed
		{"=foo", false},
		{"git", false},
		{"--repo=foo/bar", false}, // leading dash
		{"1FOO=bar", false},       // can't start with digit
		{"FOO BAR=baz", false},    // shouldn't happen — Fields would split, but be safe
		{"", false},
	}
	for _, tc := range cases {
		if got := IsEnvAssignment(tc.in); got != tc.want {
			t.Errorf("IsEnvAssignment(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
