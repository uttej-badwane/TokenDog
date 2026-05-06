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

func TestProcessClaudeInjectsSessionEnv(t *testing.T) {
	in := ClaudeHookInput{
		SessionID:      "abc-123-def",
		TranscriptPath: "/Users/me/.claude/projects/proj/sess.jsonl",
		ToolName:       "Bash",
		ToolInput:      map[string]any{"command": "git status"},
	}
	out := ProcessClaude(in)
	if out == nil {
		t.Fatal("ProcessClaude returned nil")
	}
	cmd := out.HookSpecificOutput.UpdatedInput["command"].(string)
	want := `TD_SESSION_ID=abc-123-def TD_TRANSCRIPT_PATH='/Users/me/.claude/projects/proj/sess.jsonl' td git status`
	if cmd != want {
		t.Errorf("rewritten command = %q\nwant %q", cmd, want)
	}
}

func TestProcessClaudeRejectsUnsafeSession(t *testing.T) {
	// A session id containing a shell metachar would corrupt the command —
	// drop it instead. The rewrite still happens, just without env injection.
	in := ClaudeHookInput{
		SessionID: "abc;rm -rf /",
		ToolName:  "Bash",
		ToolInput: map[string]any{"command": "git status"},
	}
	out := ProcessClaude(in)
	if out == nil {
		t.Fatal("ProcessClaude returned nil")
	}
	cmd := out.HookSpecificOutput.UpdatedInput["command"].(string)
	if cmd != "td git status" {
		t.Errorf("expected unsafe session_id to be dropped, got %q", cmd)
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
		{"bash -c double-quoted", `bash -c "git status"`, `bash -c "td git status"`},
		{"bash -c single-quoted", `bash -c 'git status'`, `bash -c 'td git status'`},
		{"sh -c", `sh -c "ls /tmp"`, `sh -c "td ls /tmp"`},
		{"zsh -c", `zsh -c "find ."`, `zsh -c "td find ."`},
		{"bash -lc login flag", `bash -lc "git log -5"`, `bash -lc "td git log -5"`},
		{"bash -c unsupported binary", `bash -c "echo hi"`, `bash -c "echo hi"`},
		{"bash -c with env-var prefix", `bash -c "AWS_PROFILE=foo aws ec2 describe-instances"`, `bash -c "AWS_PROFILE=foo td aws ec2 describe-instances"`},
		{"bash -c unterminated quote", `bash -c "git status`, `bash -c "git status`},
		{"bash without -c", `bash script.sh`, `bash script.sh`},
		{"bash -c with chain inside", `bash -c "echo 'hi' && git status"`, `bash -c "echo 'hi' && td git status"`},
		// Chain operator rewrites — biggest unhandled category in real
		// transcripts is `cd /path && supported-cmd`.
		{"chain: cd && git", "cd /tmp && git status", "cd /tmp && td git status"},
		{"chain: semicolon both supported", "git status; ls", "td git status ; td ls"},
		{"chain: multi-cd to git", "cd /a && cd /b && git log", "cd /a && cd /b && td git log"},
		{"chain: || or", "git status || echo failed", "td git status || echo failed"},
		{"chain: env-prefix on supported segment", "AWS_PROFILE=p aws s3 ls && git status", "AWS_PROFILE=p td aws s3 ls && td git status"},
		{"chain: no supported anywhere", "cd /a && cd /b && echo done", "cd /a && cd /b && echo done"},
		// Quote-aware bail: chain ops inside quotes must NOT split.
		{"quoted: && in double quotes", `echo "a && b"`, `echo "a && b"`},
		{"quoted: outer && with quoted inner", `echo 'cmd1 && cmd2' && git status`, `echo 'cmd1 && cmd2' && td git status`},
		// Escapes and substitutions — splitter must bail.
		{"escaped &&", `echo a \&\& b`, `echo a \&\& b`},
		{"backticks", "echo `git status && ls`", "echo `git status && ls`"},
		{"command sub $()", "echo $(git status; ls)", "echo $(git status; ls)"},
		{"heredoc bail", "cat <<EOF\n&&\nEOF", "cat <<EOF\n&&\nEOF"},
		// Single & is background, single | is pipe — neither should split.
		// `ls &` runs supported `ls` in the background. We rewrite ls;
		// the lone `&` doesn't trigger chain splitting (only `&&` does).
		{"background single &", "ls /tmp &", "td ls /tmp &"},
		{"pipe single |", "git log | head -5", "td git log | head -5"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RewriteCommand(tc.in); got != tc.want {
				t.Errorf("RewriteCommand(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSplitChain(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		segments []string
		seps     []string
	}{
		{"no chain", "git status", []string{"git status"}, nil},
		{"empty", "", []string{""}, nil},
		{"whitespace", "   ", []string{"   "}, nil},
		{"&&", "a && b", []string{"a", "b"}, []string{"&&"}},
		{"||", "a || b", []string{"a", "b"}, []string{"||"}},
		{";", "a; b", []string{"a", "b"}, []string{";"}},
		{"three segments", "a && b; c", []string{"a", "b", "c"}, []string{"&&", ";"}},
		{"single & (background) is not chain", "a & b", []string{"a & b"}, nil},
		{"single | (pipe) is not chain", "a | b", []string{"a | b"}, nil},
		{"quoted && stays single", `echo "a && b"`, []string{`echo "a && b"`}, nil},
		{"backtick bail", "echo `a && b`", []string{"echo `a && b`"}, nil},
		{"$() bail", "echo $(a && b)", []string{"echo $(a && b)"}, nil},
		{"heredoc bail", "cat <<EOF\n&&\nEOF", []string{"cat <<EOF\n&&\nEOF"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			segs, seps := splitChain(tc.in)
			if len(segs) != len(tc.segments) {
				t.Fatalf("segs len = %d (%v), want %d (%v)", len(segs), segs, len(tc.segments), tc.segments)
			}
			for i := range segs {
				if segs[i] != tc.segments[i] {
					t.Errorf("seg[%d] = %q, want %q", i, segs[i], tc.segments[i])
				}
			}
			if len(seps) != len(tc.seps) {
				t.Fatalf("seps len = %d (%v), want %d (%v)", len(seps), seps, len(tc.seps), tc.seps)
			}
			for i := range seps {
				if seps[i] != tc.seps[i] {
					t.Errorf("sep[%d] = %q, want %q", i, seps[i], tc.seps[i])
				}
			}
		})
	}
}

func TestInjectSessionEnvChainForm(t *testing.T) {
	// Single-segment: leading-assignment form preserved (existing behavior).
	got := injectSessionEnv("td git status", "abc-123", "/p/sess.jsonl")
	want := `TD_SESSION_ID=abc-123 TD_TRANSCRIPT_PATH='/p/sess.jsonl' td git status`
	if got != want {
		t.Errorf("single-segment form mismatch:\n  got:  %q\n  want: %q", got, want)
	}

	// Chain form: export so vars propagate across segments.
	got = injectSessionEnv("cd /tmp && td git status", "abc-123", "/p/sess.jsonl")
	want = `export TD_SESSION_ID=abc-123 TD_TRANSCRIPT_PATH='/p/sess.jsonl'; cd /tmp && td git status`
	if got != want {
		t.Errorf("chain form mismatch:\n  got:  %q\n  want: %q", got, want)
	}
}

func TestProcessClaudeChainEndToEnd(t *testing.T) {
	// Real shape Claude emits: cd into a path, then run a supported command.
	// The hook should rewrite the supported segment AND use export-form env.
	in := ClaudeHookInput{
		SessionID:      "session-abc",
		TranscriptPath: "/Users/me/.claude/projects/proj/sess.jsonl",
		ToolName:       "Bash",
		ToolInput:      map[string]any{"command": "cd /repo && git status"},
	}
	out := ProcessClaude(in)
	if out == nil {
		t.Fatal("ProcessClaude returned nil")
	}
	got := out.HookSpecificOutput.UpdatedInput["command"].(string)
	want := `export TD_SESSION_ID=session-abc TD_TRANSCRIPT_PATH='/Users/me/.claude/projects/proj/sess.jsonl'; cd /repo && td git status`
	if got != want {
		t.Errorf("chain end-to-end mismatch:\n  got:  %q\n  want: %q", got, want)
	}
}

func TestParseBinary(t *testing.T) {
	cases := []struct {
		name, in string
		wantBin  string
		wantArgs []string
		wantOK   bool
	}{
		{"plain", "git status", "git", []string{"status"}, true},
		{"path-prefixed", "/usr/local/bin/git status", "git", []string{"status"}, true},
		{"env prefix", "AWS_PROFILE=foo aws ec2 describe-instances", "aws", []string{"ec2", "describe-instances"}, true},
		{"already td", "td git status", "git", []string{"status"}, true},
		{"already tokendog", "tokendog git status", "git", []string{"status"}, true},
		{"bash -c double-quoted", `bash -c "git status"`, "git", []string{"status"}, true},
		{"bash -c single-quoted", `bash -c 'git status'`, "git", []string{"status"}, true},
		{"sh -lc env", `sh -lc "AWS_PROFILE=p aws s3 ls"`, "aws", []string{"s3", "ls"}, true},
		{"unsupported", "echo hello", "", nil, false},
		{"empty", "", "", nil, false},
		{"only env vars", "FOO=bar BAZ=qux", "", nil, false},
		// Chain handling — exactly one Supported segment is the success
		// case. Multiple supported segments deliberately return false to
		// avoid mis-attributing concatenated tool_result output.
		{"chain: one supported", "cd /path && git log", "git", []string{"log"}, true},
		{"chain: multi-cd one supported", "cd /a && cd /b && cd /c && git log", "git", []string{"log"}, true},
		{"chain: two supported skipped", "git status; ls", "", nil, false},
		{"chain: env-prefix supported", "AWS_PROFILE=p aws s3 ls && cd /x", "aws", []string{"s3", "ls"}, true},
		{"chain: zero supported", "cd /a && echo hi", "", nil, false},
		{"pipe stays single segment", "git log | head -5", "git", []string{"log", "|", "head", "-5"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bin, args, ok := ParseBinary(tc.in)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if bin != tc.wantBin {
				t.Errorf("bin = %q, want %q", bin, tc.wantBin)
			}
			if len(args) != len(tc.wantArgs) {
				t.Fatalf("args len = %d, want %d (%v vs %v)", len(args), len(tc.wantArgs), args, tc.wantArgs)
			}
			for i := range args {
				if args[i] != tc.wantArgs[i] {
					t.Errorf("args[%d] = %q, want %q", i, args[i], tc.wantArgs[i])
				}
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
