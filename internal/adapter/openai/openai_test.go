package openai

import (
	"encoding/json"
	"strings"
	"testing"

	// Blank-import the Anthropic adapter so it self-registers and Dispatch
	// can route /v1/messages to it in TestDispatchRoutesByPath.
	_ "tokendog/internal/adapter/anthropic"
	"tokendog/internal/core"
)

// TestDispatchRoutesByPath proves the registry sends each provider's path to
// the right adapter (and unknown paths through untouched) — the multi-provider
// contract a gateway frontend relies on.
func TestDispatchRoutesByPath(t *testing.T) {
	// OpenAI-shaped request.
	oaBody, _ := json.Marshal(map[string]any{
		"messages": []any{
			map[string]any{"role": "assistant", "content": nil, "tool_calls": []any{
				map[string]any{"id": "c1", "type": "function", "function": map[string]any{
					"name": "bash", "arguments": `{"command":"git status"}`,
				}},
			}},
			map[string]any{"role": "tool", "tool_call_id": "c1",
				"content": "On branch main\n\tmodified:   foo.go\n"},
		},
	})
	out, _, err := core.Dispatch("/v1/chat/completions", oaBody, core.Options{})
	if err != nil || len(out) >= len(oaBody) {
		t.Errorf("OpenAI path not compressed: err=%v %d->%d", err, len(oaBody), len(out))
	}

	// Anthropic-shaped request routed to the anthropic adapter.
	anBody, _ := json.Marshal(map[string]any{
		"messages": []any{
			map[string]any{"role": "assistant", "content": []any{map[string]any{
				"type": "tool_use", "id": "t1", "name": "Bash",
				"input": map[string]any{"command": "git status"},
			}}},
			map[string]any{"role": "user", "content": []any{map[string]any{
				"type": "tool_result", "tool_use_id": "t1",
				"content": "On branch main\n\tmodified:   foo.go\n",
			}}},
		},
	})
	out, _, err = core.Dispatch("/v1/messages", anBody, core.Options{})
	if err != nil || len(out) >= len(anBody) {
		t.Errorf("Anthropic path not compressed: err=%v %d->%d", err, len(anBody), len(out))
	}

	// Unknown path passes through unchanged.
	misc := []byte(`{"hello":"world"}`)
	out, sav, _ := core.Dispatch("/v1/models", misc, core.Options{})
	if string(out) != string(misc) || len(sav) != 0 {
		t.Error("unknown path should pass through untouched")
	}
}

func TestMatch(t *testing.T) {
	a := Adapter{}
	if !a.Match("/v1/chat/completions") || !a.Match("/chat/completions") {
		t.Error("should match chat completions paths")
	}
	if a.Match("/v1/messages") {
		t.Error("should not match the Anthropic path")
	}
}

// TestCompressFiltersLastToolMessage — an assistant `git status` tool call
// followed by a role:"tool" result message gets the result compacted.
func TestCompressFiltersLastToolMessage(t *testing.T) {
	gitStatus := "On branch main\nYour branch is up to date with 'origin/main'.\n\n" +
		"Changes not staged for commit:\n\tmodified:   foo.go\n"
	req := map[string]any{
		"model": "gpt-4o",
		"messages": []any{
			map[string]any{"role": "user", "content": "check status"},
			map[string]any{"role": "assistant", "content": nil, "tool_calls": []any{
				map[string]any{"id": "call_1", "type": "function", "function": map[string]any{
					"name": "bash", "arguments": `{"command":"git status"}`,
				}},
			}},
			map[string]any{"role": "tool", "tool_call_id": "call_1", "content": gitStatus},
		},
	}
	body, _ := json.Marshal(req)

	out, savings, err := Adapter{}.Compress(body, core.Options{Dedup: true})
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}
	if len(out) >= len(body) {
		t.Errorf("expected reduction, got %d -> %d", len(body), len(out))
	}
	content := lastMessageContent(t, out)
	if !strings.HasPrefix(content, "branch:") {
		t.Errorf("expected filtered git status, got %q", content)
	}
	if len(savings) != 1 || savings[0].Label != "git status" {
		t.Errorf("unexpected savings: %+v", savings)
	}
	// Unknown top-level fields (model) and message fields must survive.
	var doc map[string]any
	json.Unmarshal(out, &doc)
	if doc["model"] != "gpt-4o" {
		t.Error("top-level model field was dropped")
	}
}

// TestCompressDedupAcrossToolMessages — the same output returned twice; the
// last one becomes a back-reference, the earlier stays verbatim.
func TestCompressDedupAcrossToolMessages(t *testing.T) {
	dup := strings.Repeat("config payload line here\n", 40)
	req := map[string]any{
		"messages": []any{
			map[string]any{"role": "tool", "tool_call_id": "c1", "content": dup},
			map[string]any{"role": "assistant", "content": "thinking"},
			map[string]any{"role": "tool", "tool_call_id": "c2", "content": dup},
		},
	}
	body, _ := json.Marshal(req)
	out, _, _ := Adapter{}.Compress(body, core.Options{Dedup: true})
	content := lastMessageContent(t, out)
	if !strings.Contains(content, "identical to the output") {
		t.Errorf("expected dedup back-reference, got %q", content)
	}
	// The earlier copy must remain intact.
	if strings.Count(string(out), "config payload line here") == 0 {
		t.Error("earlier verbatim copy was removed")
	}
}

func TestCompressIgnoresNonToolLastMessage(t *testing.T) {
	req := map[string]any{
		"messages": []any{
			map[string]any{"role": "user", "content": "hello"},
		},
	}
	body, _ := json.Marshal(req)
	out, savings, _ := Adapter{}.Compress(body, core.Options{Dedup: true})
	if string(out) != string(body) || len(savings) != 0 {
		t.Error("a non-tool last message should be left unchanged")
	}
}

func lastMessageContent(t *testing.T, out []byte) string {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	msgs := doc["messages"].([]any)
	last := msgs[len(msgs)-1].(map[string]any)
	s, _ := last["content"].(string)
	return s
}
