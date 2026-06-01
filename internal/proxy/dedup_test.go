package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// bigText is a tool output large enough that a back-reference marker is a
// clear win (the Guard would reject deduping a tiny output).
func bigText(tag string) string {
	var b strings.Builder
	for i := 0; i < 60; i++ {
		b.WriteString(tag)
		b.WriteString(": configuration line with some descriptive payload here\n")
	}
	return b.String()
}

// TestDedupReplacesRepeatedBashOutput — a `cat config.yaml` whose output is
// byte-identical to an earlier `cat config.yaml` should be replaced by a
// back-reference marker in the last message only.
func TestDedupReplacesRepeatedBashOutput(t *testing.T) {
	t.Setenv("TD_NO_DEDUP", "")
	dup := bigText("server")

	req := mustMarshal(map[string]any{
		"messages": []any{
			// Turn 1: read the file.
			map[string]any{"role": "assistant", "content": []any{map[string]any{
				"type": "tool_use", "id": "t1", "name": "Bash",
				"input": map[string]any{"command": "cat config.yaml"},
			}}},
			map[string]any{"role": "user", "content": []any{map[string]any{
				"type": "tool_result", "tool_use_id": "t1", "content": dup,
			}}},
			// Some intervening turn.
			map[string]any{"role": "assistant", "content": "looking"},
			// Turn 2: read it again — identical output.
			map[string]any{"role": "assistant", "content": []any{map[string]any{
				"type": "tool_use", "id": "t2", "name": "Bash",
				"input": map[string]any{"command": "cat config.yaml"},
			}}},
			map[string]any{"role": "user", "content": []any{map[string]any{
				"type": "tool_result", "tool_use_id": "t2", "content": dup,
			}}},
		},
	})

	httpReq, _ := http.NewRequest("POST", "/v1/messages", bytes.NewReader(req))
	out, err := FilterHandler(httpReq, req)
	if err != nil {
		t.Fatalf("FilterHandler: %v", err)
	}
	if len(out) >= len(req) {
		t.Errorf("dedup should shrink payload, got %d -> %d", len(req), len(out))
	}

	last := lastToolResultContent(t, out)
	if !strings.Contains(last, "[td: identical to the output") {
		t.Fatalf("expected dedup marker, got: %q", trunc(last))
	}
	if !strings.Contains(last, "cat config.yaml") {
		t.Errorf("marker should name the producing command, got: %q", last)
	}
	// The earlier (first) copy must remain verbatim — only the last message
	// is allowed to change.
	if bytes.Count(out, []byte("server: configuration line")) == 0 {
		t.Error("the earlier verbatim copy was removed — back-reference would dangle")
	}
}

// TestDedupWorksForNonBashToolResult — re-reading the same file via the Read
// tool (no per-tool filter, no command) is one of the most common
// redundancies; dedup must still fire.
func TestDedupWorksForNonBashToolResult(t *testing.T) {
	t.Setenv("TD_NO_DEDUP", "")
	dup := bigText("module")

	req := mustMarshal(map[string]any{
		"messages": []any{
			map[string]any{"role": "assistant", "content": []any{map[string]any{
				"type": "tool_use", "id": "r1", "name": "Read",
				"input": map[string]any{"file_path": "/app/main.go"},
			}}},
			map[string]any{"role": "user", "content": []any{map[string]any{
				"type": "tool_result", "tool_use_id": "r1", "content": dup,
			}}},
			map[string]any{"role": "assistant", "content": []any{map[string]any{
				"type": "tool_use", "id": "r2", "name": "Read",
				"input": map[string]any{"file_path": "/app/main.go"},
			}}},
			map[string]any{"role": "user", "content": []any{map[string]any{
				"type": "tool_result", "tool_use_id": "r2", "content": dup,
			}}},
		},
	})

	httpReq, _ := http.NewRequest("POST", "/v1/messages", bytes.NewReader(req))
	out, _ := FilterHandler(httpReq, req)
	last := lastToolResultContent(t, out)
	if !strings.Contains(last, "[td: identical to the output") {
		t.Fatalf("expected dedup marker for repeated Read, got: %q", trunc(last))
	}
}

// TestDedupSkipsUniqueOutput — distinct outputs must pass through untouched.
func TestDedupSkipsUniqueOutput(t *testing.T) {
	t.Setenv("TD_NO_DEDUP", "")
	req := mustMarshal(map[string]any{
		"messages": []any{
			map[string]any{"role": "assistant", "content": []any{map[string]any{
				"type": "tool_use", "id": "u1", "name": "Read",
				"input": map[string]any{"file_path": "/a"},
			}}},
			map[string]any{"role": "user", "content": []any{map[string]any{
				"type": "tool_result", "tool_use_id": "u1", "content": bigText("alpha"),
			}}},
			map[string]any{"role": "assistant", "content": []any{map[string]any{
				"type": "tool_use", "id": "u2", "name": "Read",
				"input": map[string]any{"file_path": "/b"},
			}}},
			map[string]any{"role": "user", "content": []any{map[string]any{
				"type": "tool_result", "tool_use_id": "u2", "content": bigText("beta"),
			}}},
		},
	})
	httpReq, _ := http.NewRequest("POST", "/v1/messages", bytes.NewReader(req))
	out, _ := FilterHandler(httpReq, req)
	if bytes.Contains(out, []byte("[td: identical")) {
		t.Error("dedup fired on unique output")
	}
}

// TestDedupGuardSkipsTinyDuplicate — a tiny identical output where the marker
// would be larger than the content must not be replaced.
func TestDedupGuardSkipsTinyDuplicate(t *testing.T) {
	t.Setenv("TD_NO_DEDUP", "")
	tiny := "ok\n"
	req := mustMarshal(map[string]any{
		"messages": []any{
			map[string]any{"role": "assistant", "content": []any{map[string]any{
				"type": "tool_use", "id": "s1", "name": "Bash",
				"input": map[string]any{"command": "echo ok"},
			}}},
			map[string]any{"role": "user", "content": []any{map[string]any{
				"type": "tool_result", "tool_use_id": "s1", "content": tiny,
			}}},
			map[string]any{"role": "assistant", "content": []any{map[string]any{
				"type": "tool_use", "id": "s2", "name": "Bash",
				"input": map[string]any{"command": "echo ok"},
			}}},
			map[string]any{"role": "user", "content": []any{map[string]any{
				"type": "tool_result", "tool_use_id": "s2", "content": tiny,
			}}},
		},
	})
	httpReq, _ := http.NewRequest("POST", "/v1/messages", bytes.NewReader(req))
	out, _ := FilterHandler(httpReq, req)
	if bytes.Contains(out, []byte("[td: identical")) {
		t.Error("Guard should have blocked deduping a tiny output")
	}
}

// TestDedupDisabledByEnv — TD_NO_DEDUP=1 turns the pass off.
func TestDedupDisabledByEnv(t *testing.T) {
	t.Setenv("TD_NO_DEDUP", "1")
	dup := bigText("svc")
	req := mustMarshal(map[string]any{
		"messages": []any{
			map[string]any{"role": "assistant", "content": []any{map[string]any{
				"type": "tool_use", "id": "d1", "name": "Read",
				"input": map[string]any{"file_path": "/x"},
			}}},
			map[string]any{"role": "user", "content": []any{map[string]any{
				"type": "tool_result", "tool_use_id": "d1", "content": dup,
			}}},
			map[string]any{"role": "assistant", "content": []any{map[string]any{
				"type": "tool_use", "id": "d2", "name": "Read",
				"input": map[string]any{"file_path": "/x"},
			}}},
			map[string]any{"role": "user", "content": []any{map[string]any{
				"type": "tool_result", "tool_use_id": "d2", "content": dup,
			}}},
		},
	})
	httpReq, _ := http.NewRequest("POST", "/v1/messages", bytes.NewReader(req))
	out, _ := FilterHandler(httpReq, req)
	if bytes.Contains(out, []byte("[td: identical")) {
		t.Error("dedup fired despite TD_NO_DEDUP=1")
	}
}

// lastToolResultContent extracts the string content of the first tool_result
// block in the last message of a serialized request.
func lastToolResultContent(t *testing.T, out []byte) string {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	msgs := doc["messages"].([]any)
	last := msgs[len(msgs)-1].(map[string]any)
	for _, raw := range last["content"].([]any) {
		blk := raw.(map[string]any)
		if blk["type"] == "tool_result" {
			s, _ := blk["content"].(string)
			return s
		}
	}
	t.Fatal("no tool_result in last message")
	return ""
}

func trunc(s string) string {
	if len(s) > 120 {
		return s[:120]
	}
	return s
}
