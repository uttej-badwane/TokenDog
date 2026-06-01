// Package anthropic adapts the Anthropic Messages API wire format to the
// provider-neutral core engine. It owns everything Anthropic-specific —
// request/content-block parsing, the tool_use→command lookup, tool-description
// compression, and writing replacements back while preserving unknown fields —
// so the engine itself stays format-agnostic.
package anthropic

import (
	"encoding/json"
	"strings"

	"tokendog/internal/compress"
	"tokendog/internal/core"
)

func init() { core.Register(Adapter{}) }

// Adapter implements core.Provider for the Anthropic Messages API.
type Adapter struct{}

func (Adapter) Name() string { return "anthropic" }

func (Adapter) Match(path string) bool { return path == "/v1/messages" }

// Compress parses an Anthropic Messages request, compresses tool_result
// blocks in the LAST message only (cache safety) plus tool descriptions, and
// re-serializes. On any parse/marshal trouble it returns the body unchanged —
// TokenDog never sends a payload it couldn't faithfully round-trip.
func (Adapter) Compress(body []byte, opts core.Options) ([]byte, []core.Saving, error) {
	if len(body) == 0 {
		return body, nil, nil
	}
	var doc messagesRequest
	if err := json.Unmarshal(body, &doc); err != nil {
		return body, nil, nil
	}
	if len(doc.Messages) == 0 {
		return body, nil, nil
	}

	// tool_use_id → producing command, across all messages.
	useByID := map[string]string{}
	for _, m := range doc.Messages {
		blocks, _ := unmarshalContent(m.Content)
		for _, b := range blocks {
			if b.Type == "tool_use" && b.ID != "" {
				if cmd := extractCommand(b.Input); cmd != "" {
					useByID[b.ID] = cmd
				}
			}
		}
	}

	var savings []core.Saving
	toolSaving, toolsModified := compressToolDescriptions(&doc)
	if toolsModified {
		savings = append(savings, toolSaving)
	}

	// Build the neutral Conversation: earlier messages contribute
	// non-eligible results (for the dedup index); the last message's results
	// are eligible and carry write-back handles.
	conv := &core.Conversation{}
	for _, m := range doc.Messages[:len(doc.Messages)-1] {
		blocks, ok := unmarshalContent(m.Content)
		if !ok {
			continue
		}
		for i := range blocks {
			b := &blocks[i]
			if b.Type != "tool_result" || b.ToolUseID == "" {
				continue
			}
			if txt := extractText(b.Content); txt != "" {
				conv.Results = append(conv.Results, &core.ToolResult{
					Command: useByID[b.ToolUseID], Text: txt, Eligible: false,
				})
			}
		}
	}

	last := &doc.Messages[len(doc.Messages)-1]
	lastBlocks, contentOK := unmarshalContent(last.Content)
	var eligible []*core.ToolResult
	var handles []*contentBlock
	if contentOK {
		for i := range lastBlocks {
			b := &lastBlocks[i]
			if b.Type != "tool_result" || b.ToolUseID == "" {
				continue
			}
			txt := extractText(b.Content)
			if txt == "" {
				continue
			}
			tr := &core.ToolResult{Command: useByID[b.ToolUseID], Text: txt, Eligible: true}
			conv.Results = append(conv.Results, tr)
			eligible = append(eligible, tr)
			handles = append(handles, b)
		}
	}

	savings = append(savings, core.Compress(conv, opts)...)

	// Apply replacements back into the last message's blocks.
	modified := false
	for k, tr := range eligible {
		if tr.Replaced {
			handles[k].Content = json.RawMessage(mustMarshalString(tr.Replacement))
			modified = true
		}
	}
	if modified {
		last.Content = mustMarshalContentBlocks(lastBlocks)
	}

	if !modified && !toolsModified {
		return body, savings, nil
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return body, nil, nil
	}
	return out, savings, nil
}

// compressToolDescriptions compresses description fields in doc.Tools. Returns
// the saving + whether anything changed. Skips the whole tools array if any
// tool carries cache_control (preserves the cached payload).
func compressToolDescriptions(doc *messagesRequest) (core.Saving, bool) {
	if len(doc.Tools) == 0 {
		return core.Saving{}, false
	}
	var tools []map[string]json.RawMessage
	if err := json.Unmarshal(doc.Tools, &tools); err != nil {
		return core.Saving{}, false
	}
	for _, t := range tools {
		if cc, ok := t["cache_control"]; ok && len(cc) > 0 && string(cc) != "null" {
			return core.Saving{}, false
		}
	}

	modified := false
	for i, t := range tools {
		raw, ok := t["description"]
		if !ok || len(raw) == 0 {
			continue
		}
		var desc string
		if err := json.Unmarshal(raw, &desc); err != nil || desc == "" {
			continue
		}
		compressed := compress.CompressString(desc)
		if compressed == desc {
			continue
		}
		newRaw, err := json.Marshal(compressed)
		if err != nil {
			continue
		}
		tools[i]["description"] = newRaw
		modified = true
	}
	if !modified {
		return core.Saving{}, false
	}
	newTools, err := json.Marshal(tools)
	if err != nil {
		return core.Saving{}, false
	}
	before := string(doc.Tools)
	doc.Tools = newTools
	return core.Saving{Label: "tools/descriptions", Original: before, Result: string(newTools)}, true
}

// --- Anthropic wire shapes (only what the adapter needs; unknown fields
// round-trip via json.RawMessage). ---

type messagesRequest struct {
	Model         json.RawMessage `json:"model,omitempty"`
	MaxTokens     json.RawMessage `json:"max_tokens,omitempty"`
	System        json.RawMessage `json:"system,omitempty"`
	Tools         json.RawMessage `json:"tools,omitempty"`
	Messages      []messageEntry  `json:"messages"`
	Stream        json.RawMessage `json:"stream,omitempty"`
	Temperature   json.RawMessage `json:"temperature,omitempty"`
	TopP          json.RawMessage `json:"top_p,omitempty"`
	StopSequences json.RawMessage `json:"stop_sequences,omitempty"`
	Metadata      json.RawMessage `json:"metadata,omitempty"`
}

type messageEntry struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type contentBlock struct {
	Type         string          `json:"type"`
	ID           string          `json:"id,omitempty"`
	Name         string          `json:"name,omitempty"`
	Input        json.RawMessage `json:"input,omitempty"`
	ToolUseID    string          `json:"tool_use_id,omitempty"`
	Content      json.RawMessage `json:"content,omitempty"`
	Text         string          `json:"text,omitempty"`
	IsError      *bool           `json:"is_error,omitempty"`
	CacheControl json.RawMessage `json:"cache_control,omitempty"`
}

// unmarshalContent handles both shapes Anthropic uses for message content: a
// bare string or an array of content blocks. ok=false means it was a string
// (no tool_result blocks to compress).
func unmarshalContent(raw json.RawMessage) ([]contentBlock, bool) {
	if len(raw) == 0 || raw[0] != '[' {
		return nil, false
	}
	var blocks []contentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, false
	}
	return blocks, true
}

// extractCommand pulls a Bash tool_use's "command" field. Non-Bash tools have
// no command and yield "".
func extractCommand(input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}
	var probe struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(input, &probe); err != nil {
		return ""
	}
	return probe.Command
}

// extractText pulls text out of a tool_result block — either a bare string or
// an array of {type:"text", text:"..."} sub-blocks.
func extractText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var sub []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &sub); err == nil {
		var b strings.Builder
		for _, blk := range sub {
			if blk.Type == "text" {
				b.WriteString(blk.Text)
			}
		}
		return b.String()
	}
	return ""
}

func mustMarshalString(s string) []byte {
	out, err := json.Marshal(s)
	if err != nil {
		return []byte(`""`)
	}
	return out
}

func mustMarshalContentBlocks(blocks []contentBlock) []byte {
	out, err := json.Marshal(blocks)
	if err != nil {
		return []byte("[]")
	}
	return out
}
