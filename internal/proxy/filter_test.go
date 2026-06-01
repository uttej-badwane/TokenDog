package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"tokendog/internal/stash"
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

// TestFilterHandlerCompressesToolDescriptions — tool descriptions without
// cache_control should be compressed by the proxy.
func TestFilterHandlerCompressesToolDescriptions(t *testing.T) {
	req := mustMarshal(map[string]any{
		"model": "claude-sonnet-4-6",
		"tools": []any{
			map[string]any{
				"name":        "bash",
				"description": "Run commands in a bash shell. Please make sure to just use this tool when you basically need to execute a command. You should always provide the full command.",
				"input_schema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"command": map[string]any{"type": "string"},
					},
				},
			},
		},
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": "hello",
			},
		},
	})

	httpReq, _ := http.NewRequest("POST", "/v1/messages", bytes.NewReader(req))
	out, err := FilterHandler(httpReq, req)
	if err != nil {
		t.Fatalf("FilterHandler: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	tools := doc["tools"].([]any)
	tool := tools[0].(map[string]any)
	desc := tool["description"].(string)

	// Fillers like "please", "basically", "just" should be gone.
	// "always" is a semantic qualifier and is intentionally kept.
	for _, filler := range []string{"please", "basically", "just"} {
		if strings.Contains(strings.ToLower(desc), filler) {
			t.Errorf("filler %q still present in tool description: %q", filler, desc)
		}
	}
	if len(out) >= len(req) {
		t.Errorf("expected payload reduction from tool compression: %d -> %d", len(req), len(out))
	}
}

// TestFilterHandlerSkipsToolCompressionWithCacheControl — tools bearing
// cache_control must not be touched (would invalidate the warm cache entry).
func TestFilterHandlerSkipsToolCompressionWithCacheControl(t *testing.T) {
	req := mustMarshal(map[string]any{
		"model": "claude-sonnet-4-6",
		"tools": []any{
			map[string]any{
				"name":          "bash",
				"description":   "Run commands in a bash shell. Please just use this when you basically need a command.",
				"cache_control": map[string]any{"type": "ephemeral"},
				"input_schema":  map[string]any{"type": "object"},
			},
		},
		"messages": []any{
			map[string]any{"role": "user", "content": "hi"},
		},
	})

	httpReq, _ := http.NewRequest("POST", "/v1/messages", bytes.NewReader(req))
	out, _ := FilterHandler(httpReq, req)

	// Tools should be untouched when cache_control is present.
	var orig, got map[string]any
	json.Unmarshal(req, &orig)
	json.Unmarshal(out, &got)

	origDesc := orig["tools"].([]any)[0].(map[string]any)["description"].(string)
	gotDesc := got["tools"].([]any)[0].(map[string]any)["description"].(string)
	if origDesc != gotDesc {
		t.Errorf("tool description was modified despite cache_control:\n  orig: %q\n  got:  %q", origDesc, gotDesc)
	}
}

// TestFilterHandlerReversiblePass — with TD_REVERSIBLE=1, a large tool_result
// (here from an unsupported binary, so no lossless filter applies) is replaced
// by a compact preview carrying a [td:STASHED id=…] marker, and the full
// original is recoverable from the stash by that id.
func TestFilterHandlerReversiblePass(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TD_REVERSIBLE", "1")

	var lines []string
	for i := 0; i < 400; i++ {
		lines = append(lines, "journalctl line "+strconv.Itoa(i)+" with some payload text")
	}
	big := strings.Join(lines, "\n")

	req := mustMarshal(map[string]any{
		"messages": []any{
			map[string]any{
				"role": "assistant",
				"content": []any{map[string]any{
					"type": "tool_use", "id": "toolu_rev", "name": "Bash",
					"input": map[string]any{"command": "journalctl -u nginx"},
				}},
			},
			map[string]any{
				"role": "user",
				"content": []any{map[string]any{
					"type": "tool_result", "tool_use_id": "toolu_rev",
					"content": big,
				}},
			},
		},
	})

	httpReq, _ := http.NewRequest("POST", "/v1/messages", bytes.NewReader(req))
	out, err := FilterHandler(httpReq, req)
	if err != nil {
		t.Fatalf("FilterHandler: %v", err)
	}
	if len(out) >= len(req) {
		t.Errorf("reversible pass should shrink payload, got %d -> %d", len(req), len(out))
	}

	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	msgs := doc["messages"].([]any)
	last := msgs[len(msgs)-1].(map[string]any)
	content := last["content"].([]any)[0].(map[string]any)["content"].(string)

	if !strings.Contains(content, "td:STASHED id=") {
		t.Fatalf("expected stash marker in preview, got: %q", content[:min(120, len(content))])
	}
	// Pull the id back out of the marker and confirm the original is recoverable.
	id := stash.ID(big)
	rec, ok := stash.Get(id)
	if !ok {
		t.Fatalf("original not retrievable from stash for id %s", id)
	}
	if rec.Content != big {
		t.Error("stashed content does not match original")
	}
}

// TestFilterHandlerReversibleOffByDefault — without the opt-in env var, a
// large output is left to the lossless path (no stash marker injected).
func TestFilterHandlerReversibleOffByDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TD_REVERSIBLE", "")

	big := strings.Repeat("journalctl noise line here\n", 400)
	req := mustMarshal(map[string]any{
		"messages": []any{
			map[string]any{
				"role": "assistant",
				"content": []any{map[string]any{
					"type": "tool_use", "id": "toolu_off", "name": "Bash",
					"input": map[string]any{"command": "journalctl -u nginx"},
				}},
			},
			map[string]any{
				"role": "user",
				"content": []any{map[string]any{
					"type": "tool_result", "tool_use_id": "toolu_off",
					"content": big,
				}},
			},
		},
	})
	httpReq, _ := http.NewRequest("POST", "/v1/messages", bytes.NewReader(req))
	out, _ := FilterHandler(httpReq, req)
	if bytes.Contains(out, []byte("td:STASHED")) {
		t.Error("reversible pass fired despite TD_REVERSIBLE unset")
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
