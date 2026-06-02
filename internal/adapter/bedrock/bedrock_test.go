package bedrock

import (
	"encoding/json"
	"strings"
	"testing"

	"tokendog/internal/core"
)

func TestMatch(t *testing.T) {
	a := Adapter{}
	for _, p := range []string{
		"/model/anthropic.claude-3-5-sonnet/converse",
		"/model/meta.llama3/converse-stream",
	} {
		if !a.Match(p) {
			t.Errorf("should match %q", p)
		}
	}
	if a.Match("/v1/messages") || a.Match("/v1/chat/completions") {
		t.Error("should not match other providers' paths")
	}
}

// TestCompressFiltersLastToolResult — a Converse request whose last message
// carries a `git status` toolResult gets that result compacted, with all other
// fields and messages preserved.
func TestCompressFiltersLastToolResult(t *testing.T) {
	gitStatus := "On branch main\nChanges not staged for commit:\n\tmodified:   handler.go\n"
	req := map[string]any{
		"inferenceConfig": map[string]any{"maxTokens": 1024},
		"messages": []any{
			map[string]any{"role": "user", "content": []any{
				map[string]any{"text": "what's the status?"},
			}},
			map[string]any{"role": "assistant", "content": []any{
				map[string]any{"toolUse": map[string]any{
					"toolUseId": "tu_1", "name": "bash",
					"input": map[string]any{"command": "git status"},
				}},
			}},
			map[string]any{"role": "user", "content": []any{
				map[string]any{"toolResult": map[string]any{
					"toolUseId": "tu_1",
					"content":   []any{map[string]any{"text": gitStatus}},
					"status":    "success",
				}},
			}},
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

	text := lastToolResultText(t, out)
	if !strings.HasPrefix(text, "branch:") {
		t.Errorf("expected filtered git status, got %q", text)
	}
	if !strings.Contains(text, "handler.go") {
		t.Errorf("filter dropped the filename: %q", text)
	}
	if len(savings) != 1 || savings[0].Label != "git status" {
		t.Errorf("unexpected savings: %+v", savings)
	}

	// Preserved fields: inferenceConfig, the toolResult status, and the user text.
	var doc map[string]any
	json.Unmarshal(out, &doc)
	if _, ok := doc["inferenceConfig"]; !ok {
		t.Error("inferenceConfig was dropped")
	}
	if !strings.Contains(string(out), `"status":"success"`) && !strings.Contains(string(out), `"status": "success"`) {
		t.Error("toolResult status field was dropped")
	}
	if !strings.Contains(string(out), "what's the status?") {
		t.Error("earlier user text was dropped")
	}
}

func TestCompressDedupAcrossMessages(t *testing.T) {
	dup := strings.Repeat("config payload line\n", 40)
	mkToolResult := func(id, text string) map[string]any {
		return map[string]any{"role": "user", "content": []any{
			map[string]any{"toolResult": map[string]any{
				"toolUseId": id, "content": []any{map[string]any{"text": text}},
			}},
		}}
	}
	req := map[string]any{
		"messages": []any{
			mkToolResult("a", dup),
			map[string]any{"role": "assistant", "content": []any{map[string]any{"text": "ok"}}},
			mkToolResult("b", dup),
		},
	}
	body, _ := json.Marshal(req)
	out, _, _ := Adapter{}.Compress(body, core.Options{Dedup: true})
	if !strings.Contains(lastToolResultText(t, out), "identical to the output") {
		t.Error("expected dedup back-reference in the last toolResult")
	}
	if strings.Count(string(out), "config payload line") == 0 {
		t.Error("earlier verbatim copy was removed")
	}
}

func TestCompressIgnoresNonToolLastMessage(t *testing.T) {
	req := map[string]any{
		"messages": []any{
			map[string]any{"role": "user", "content": []any{map[string]any{"text": "hi"}}},
		},
	}
	body, _ := json.Marshal(req)
	out, savings, _ := Adapter{}.Compress(body, core.Options{Dedup: true})
	if string(out) != string(body) || len(savings) != 0 {
		t.Error("a message with no tool result should be unchanged")
	}
}

// lastToolResultText digs out the text of the first toolResult in the last
// message of a Converse request.
func lastToolResultText(t *testing.T, out []byte) string {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	msgs := doc["messages"].([]any)
	last := msgs[len(msgs)-1].(map[string]any)
	for _, raw := range last["content"].([]any) {
		blk := raw.(map[string]any)
		tr, ok := blk["toolResult"].(map[string]any)
		if !ok {
			continue
		}
		content := tr["content"].([]any)
		first := content[0].(map[string]any)
		s, _ := first["text"].(string)
		return s
	}
	t.Fatal("no toolResult in last message")
	return ""
}
