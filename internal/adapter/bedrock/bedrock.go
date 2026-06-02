// Package bedrock adapts the Amazon Bedrock Converse API to the
// provider-neutral core engine, so `td gateway --upstream
// https://bedrock-runtime.<region>.amazonaws.com` can compress Bedrock traffic
// the same way it does Anthropic and OpenAI. This is the "Bedrock middleware"
// deployment: drop the gateway in front of Bedrock, no SDK changes beyond the
// endpoint.
//
// Bedrock Converse nests tool results deeper than the other providers: a
// tool result is a `{"toolResult": {"toolUseId", "content":[{"text"}], …}}`
// block inside a user message's content array, and the producing command lives
// in an assistant message's `{"toolUse": {"toolUseId","name","input"}}` block.
// Everything is parsed through generic maps so the many Bedrock-specific
// fields (status, guardContent, inferenceConfig, …) round-trip untouched, and
// only messages we actually rewrite are re-serialized.
package bedrock

import (
	"encoding/json"
	"strings"

	"tokendog/internal/core"
)

func init() { core.Register(Adapter{}) }

// Adapter implements core.Provider for the Bedrock Converse API.
type Adapter struct{}

func (Adapter) Name() string { return "bedrock" }

// Match accepts the Converse and ConverseStream paths. The model id is part of
// the path (/model/{modelId}/converse), so match the suffix.
func (Adapter) Match(path string) bool {
	return strings.HasSuffix(path, "/converse") || strings.HasSuffix(path, "/converse-stream")
}

// Compress compresses tool results in the LAST message (cache safety).
// Anything unparseable returns the body unchanged.
func (Adapter) Compress(body []byte, opts core.Options) ([]byte, []core.Saving, error) {
	if len(body) == 0 {
		return body, nil, nil
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(body, &doc); err != nil {
		return body, nil, nil
	}
	rawMsgs, ok := doc["messages"]
	if !ok {
		return body, nil, nil
	}
	var msgs []json.RawMessage
	if err := json.Unmarshal(rawMsgs, &msgs); err != nil || len(msgs) == 0 {
		return body, nil, nil
	}

	// Parse each message into a map (preserves role + any extra fields).
	msgMaps := make([]map[string]json.RawMessage, len(msgs))
	blocksPerMsg := make([][]map[string]json.RawMessage, len(msgs))
	for i, m := range msgs {
		_ = json.Unmarshal(m, &msgMaps[i])
		blocksPerMsg[i] = parseBlocks(msgMaps[i]["content"])
	}

	// toolUseId → producing command, from assistant toolUse blocks.
	cmdByID := map[string]string{}
	for _, blocks := range blocksPerMsg {
		for _, b := range blocks {
			tuRaw, ok := b["toolUse"]
			if !ok {
				continue
			}
			var tu struct {
				ToolUseID string `json:"toolUseId"`
				Input     struct {
					Command string `json:"command"`
				} `json:"input"`
			}
			if json.Unmarshal(tuRaw, &tu) == nil && tu.Input.Command != "" {
				cmdByID[tu.ToolUseID] = tu.Input.Command
			}
		}
	}

	// Build the Conversation; only the last message's tool results are
	// eligible. Remember (messageIndex, blockIndex) for write-back.
	conv := &core.Conversation{}
	type loc struct{ mi, bi int }
	var eligible []*core.ToolResult
	var locs []loc
	lastIdx := len(msgMaps) - 1
	for mi, blocks := range blocksPerMsg {
		for bi, b := range blocks {
			trRaw, ok := b["toolResult"]
			if !ok {
				continue
			}
			txt, toolUseID := toolResultText(trRaw)
			if txt == "" {
				continue
			}
			tr := &core.ToolResult{
				Command: cmdByID[toolUseID], Text: txt, Eligible: mi == lastIdx,
			}
			conv.Results = append(conv.Results, tr)
			if mi == lastIdx {
				eligible = append(eligible, tr)
				locs = append(locs, loc{mi, bi})
			}
		}
	}

	savings := core.Compress(conv, opts)

	// Write replacements back into the toolResult content, marking which
	// messages changed so only those are re-serialized.
	changed := map[int]bool{}
	for k, tr := range eligible {
		if !tr.Replaced {
			continue
		}
		l := locs[k]
		block := blocksPerMsg[l.mi][l.bi]
		var trMap map[string]json.RawMessage
		if json.Unmarshal(block["toolResult"], &trMap) != nil {
			continue
		}
		trMap["content"] = mustMarshal([]map[string]string{{"text": tr.Replacement}})
		block["toolResult"] = mustMarshal(trMap)
		changed[l.mi] = true
	}
	if len(changed) == 0 {
		return body, savings, nil
	}

	for mi := range changed {
		msgMaps[mi]["content"] = mustMarshal(blocksPerMsg[mi])
		msgs[mi] = mustMarshal(msgMaps[mi])
	}
	doc["messages"] = mustMarshal(msgs)
	out, err := json.Marshal(doc)
	if err != nil {
		return body, nil, nil
	}
	return out, savings, nil
}

// parseBlocks parses a message's content array into per-block maps. Returns
// nil when content isn't an array of objects (e.g. absent), which the caller
// treats as "no tool results here".
func parseBlocks(raw json.RawMessage) []map[string]json.RawMessage {
	if len(raw) == 0 || raw[0] != '[' {
		return nil
	}
	var blocks []map[string]json.RawMessage
	if json.Unmarshal(raw, &blocks) != nil {
		return nil
	}
	return blocks
}

// toolResultText extracts the concatenated text from a toolResult block and
// its toolUseId. Non-text content (image/json blocks) contributes nothing, so
// a toolResult with no text yields "".
func toolResultText(raw json.RawMessage) (text, toolUseID string) {
	var tr struct {
		ToolUseID string `json:"toolUseId"`
		Content   []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(raw, &tr) != nil {
		return "", ""
	}
	var b strings.Builder
	for _, c := range tr.Content {
		b.WriteString(c.Text)
	}
	return b.String(), tr.ToolUseID
}

func mustMarshal(v any) json.RawMessage {
	out, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("null")
	}
	return out
}
