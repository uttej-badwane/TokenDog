package hook

import (
	"testing"
)

// BenchmarkProcessClaudeSimple measures the most common hot path: a plain
// supported binary with no chain, no bash -c, no env-var prefix. This runs
// before every Bash tool call Claude makes; budget is single-digit
// microseconds (we want to be invisible to the user).
func BenchmarkProcessClaudeSimple(b *testing.B) {
	in := ClaudeHookInput{
		SessionID:      "abc-123-def",
		TranscriptPath: "/Users/me/.claude/projects/proj/sess.jsonl",
		ToolName:       "Bash",
		ToolInput:      map[string]any{"command": "git status"},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ProcessClaude(in)
	}
}

// BenchmarkProcessClaudeChain — the second-most common path: cd /path &&
// supported-cmd. Chain parsing must stay fast.
func BenchmarkProcessClaudeChain(b *testing.B) {
	in := ClaudeHookInput{
		SessionID:      "abc-123-def",
		TranscriptPath: "/Users/me/.claude/projects/proj/sess.jsonl",
		ToolName:       "Bash",
		ToolInput:      map[string]any{"command": "cd /var/log && grep ERROR /tmp/x.log && git status"},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ProcessClaude(in)
	}
}

// BenchmarkProcessClaudeBashC — bash -c "<inner>" path. Uncommon but worth
// monitoring since splitChain + unwrapShellC + recursion can compound.
func BenchmarkProcessClaudeBashC(b *testing.B) {
	in := ClaudeHookInput{
		SessionID:      "abc-123-def",
		TranscriptPath: "/Users/me/.claude/projects/proj/sess.jsonl",
		ToolName:       "Bash",
		ToolInput:      map[string]any{"command": `bash -c "AWS_PROFILE=prod aws ec2 describe-instances && echo done"`},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ProcessClaude(in)
	}
}

// BenchmarkProcessClaudeUnsupported — the early-exit path: a binary not in
// Supported. Must be cheap because it runs on every Bash call too, and the
// majority of those (in some workflows) don't get rewritten.
func BenchmarkProcessClaudeUnsupported(b *testing.B) {
	in := ClaudeHookInput{
		ToolName:  "Bash",
		ToolInput: map[string]any{"command": "echo hello world"},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ProcessClaude(in)
	}
}

func BenchmarkSplitChain(b *testing.B) {
	cmd := "AWS_PROFILE=p aws ec2 describe-instances && cd /tmp && grep ERROR /var/log/syslog"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = splitChain(cmd)
	}
}

func BenchmarkParseBinary(b *testing.B) {
	cmd := "cd /tmp && AWS_PROFILE=p aws ec2 describe-instances"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = ParseBinary(cmd)
	}
}
