package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestFilterHandlerReplacesToolResult is the core behavioral test: a
// minimal Anthropic Messages API request with one Bash tool_use + matching
// tool_result should come out with the tool_result content compressed by
// the appropriate filter.
func TestFilterHandlerReplacesToolResult(t *testing.T) {
	// Synthetic request: one assistant turn issuing `git status`, one
	// user turn with the tool_result containing realistic verbose output.
	req := mustMarshal(map[string]any{
		"model": "claude-opus-4-7",
		"messages": []any{
			map[string]any{
				"role": "assistant",
				"content": []any{
					map[string]any{
						"type":  "tool_use",
						"id":    "toolu_xyz",
						"name":  "Bash",
						"input": map[string]any{"command": "git status"},
					},
				},
			},
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{
						"type":        "tool_result",
						"tool_use_id": "toolu_xyz",
						"content":     "On branch main\nYour branch is up to date with 'origin/main'.\n\nChanges not staged for commit:\n  (use \"git add <file>...\" to update what will be committed)\n  (use \"git restore <file>...\" to discard changes in working directory)\n\tmodified:   foo.go\n\tmodified:   bar.go\n\nno changes added to commit (use \"git add\" and/or \"git commit -a\")\n",
					},
				},
			},
		},
	})

	httpReq, _ := http.NewRequest("POST", "/v1/messages", bytes.NewReader(req))
	out, err := FilterHandler(httpReq, req)
	if err != nil {
		t.Fatalf("FilterHandler: %v", err)
	}
	if len(out) >= len(req) {
		t.Errorf("expected payload reduction, got %d -> %d bytes", len(req), len(out))
	}
	// Verify the tool_result content was actually filtered (compact form
	// of the git status output).
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	msgs := doc["messages"].([]any)
	last := msgs[len(msgs)-1].(map[string]any)
	blocks := last["content"].([]any)
	tr := blocks[0].(map[string]any)
	content, _ := tr["content"].(string)
	if content == "" {
		t.Fatalf("tool_result content not a string: %v", tr["content"])
	}
	// Filtered git status: starts with "branch:" not "On branch".
	if !strings.HasPrefix(content, "branch:") {
		t.Errorf("expected filtered git status, got: %q", content[:min(80, len(content))])
	}
}

// TestFilterHandlerSkipsNonAnthropicPaths — the proxy MITMs only the
// /v1/messages endpoint. Anything else (model list, etc.) passes
// through untouched.
func TestFilterHandlerSkipsNonAnthropicPaths(t *testing.T) {
	body := []byte(`{"foo":"bar"}`)
	httpReq, _ := http.NewRequest("GET", "/v1/models", bytes.NewReader(body))
	out, err := FilterHandler(httpReq, body)
	if err != nil {
		t.Fatalf("FilterHandler: %v", err)
	}
	if !bytes.Equal(out, body) {
		t.Errorf("non-/v1/messages path should pass through, got %q", out)
	}
}

// TestFilterHandlerLeavesOlderMessagesAlone — cache-safety: only the
// LAST message gets filtered. Earlier messages are presumed cached and
// must not change.
func TestFilterHandlerLeavesOlderMessagesAlone(t *testing.T) {
	const olderContent = "ON BRANCH FOO\nWILL NOT BE TOUCHED\n"
	req := mustMarshal(map[string]any{
		"messages": []any{
			map[string]any{
				"role": "assistant",
				"content": []any{map[string]any{
					"type": "tool_use", "id": "toolu_old", "name": "Bash",
					"input": map[string]any{"command": "git status"},
				}},
			},
			map[string]any{
				"role": "user",
				"content": []any{map[string]any{
					"type": "tool_result", "tool_use_id": "toolu_old",
					"content": olderContent,
				}},
			},
			map[string]any{
				"role": "assistant",
				"content": []any{map[string]any{
					"type": "tool_use", "id": "toolu_new", "name": "Bash",
					"input": map[string]any{"command": "git status"},
				}},
			},
			map[string]any{
				"role": "user",
				"content": []any{map[string]any{
					"type": "tool_result", "tool_use_id": "toolu_new",
					"content": "On branch new\n\tmodified: x.go\n",
				}},
			},
		},
	})
	httpReq, _ := http.NewRequest("POST", "/v1/messages", bytes.NewReader(req))
	out, _ := FilterHandler(httpReq, req)
	// JSON-encoded form of the older content (newlines become \n etc.).
	jsonEncoded, _ := json.Marshal(olderContent)
	if !bytes.Contains(out, bytes.Trim(jsonEncoded, `"`)) {
		t.Errorf("older tool_result was modified — cache invalidation risk\n%s", out)
	}
}

// TestFilterHandlerSkipsUnknownBinary — a tool_use for a binary not in
// hook.Supported (e.g. `echo`) shouldn't filter the corresponding
// tool_result. Filter would no-op anyway, but we should bail before
// re-marshalling for performance.
func TestFilterHandlerSkipsUnknownBinary(t *testing.T) {
	req := mustMarshal(map[string]any{
		"messages": []any{
			map[string]any{
				"role": "assistant",
				"content": []any{map[string]any{
					"type": "tool_use", "id": "toolu_x", "name": "Bash",
					"input": map[string]any{"command": "echo hello"},
				}},
			},
			map[string]any{
				"role": "user",
				"content": []any{map[string]any{
					"type": "tool_result", "tool_use_id": "toolu_x",
					"content": "hello\n",
				}},
			},
		},
	})
	httpReq, _ := http.NewRequest("POST", "/v1/messages", bytes.NewReader(req))
	out, _ := FilterHandler(httpReq, req)
	if !bytes.Equal(out, req) {
		t.Errorf("echo (unsupported) should leave request unchanged")
	}
}

func mustMarshal(v any) []byte {
	out, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
