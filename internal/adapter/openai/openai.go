// Package openai adapts the OpenAI Chat Completions wire format to the
// provider-neutral core engine. It exists to prove the engine is genuinely
// decoupled from Anthropic — the same dedup / filter / generic / reversible
// passes run unchanged; only the parsing and write-back differ.
//
// OpenAI differs from Anthropic in two ways that matter here: tool results are
// their own messages (role:"tool", linked by tool_call_id) rather than blocks
// inside one user message, and the producing command lives in an assistant
// message's tool_calls[].function. Messages are parsed as generic maps so
// every field — the many OpenAI-specific ones we don't model — round-trips
// untouched.
package openai

import (
	"encoding/json"

	"tokendog/internal/core"
)

func init() { core.Register(Adapter{}) }

// Adapter implements core.Provider for the OpenAI Chat Completions API.
type Adapter struct{}

func (Adapter) Name() string { return "openai" }

func (Adapter) Match(path string) bool {
	return path == "/v1/chat/completions" || path == "/chat/completions"
}

// Compress compresses the tool result in the LAST message (cache safety) when
// it is a role:"tool" message. Anything unparseable returns the body
// unchanged.
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
	var msgs []map[string]json.RawMessage
	if err := json.Unmarshal(rawMsgs, &msgs); err != nil || len(msgs) == 0 {
		return body, nil, nil
	}

	// tool_call_id → producing command, from assistant tool_calls.
	cmdByID := map[string]string{}
	for _, m := range msgs {
		if str(m["role"]) != "assistant" {
			continue
		}
		tcRaw, ok := m["tool_calls"]
		if !ok {
			continue
		}
		var calls []struct {
			ID       string `json:"id"`
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		}
		if err := json.Unmarshal(tcRaw, &calls); err != nil {
			continue
		}
		for _, c := range calls {
			if cmd := commandFromArgs(c.Function.Arguments); cmd != "" {
				cmdByID[c.ID] = cmd
			}
		}
	}

	// Build the Conversation: every tool message is a result; only the last
	// message (if it's a tool message) is eligible for replacement.
	conv := &core.Conversation{}
	lastIdx := len(msgs) - 1
	var eligible *core.ToolResult
	eligIdx := -1
	for i, m := range msgs {
		if str(m["role"]) != "tool" {
			continue
		}
		txt := contentText(m["content"])
		if txt == "" {
			continue
		}
		tr := &core.ToolResult{
			Command:  cmdByID[str(m["tool_call_id"])],
			Text:     txt,
			Eligible: i == lastIdx,
		}
		conv.Results = append(conv.Results, tr)
		if i == lastIdx {
			eligible, eligIdx = tr, i
		}
	}

	savings := core.Compress(conv, opts)

	if eligible == nil || !eligible.Replaced {
		return body, savings, nil
	}
	msgs[eligIdx]["content"] = mustMarshalString(eligible.Replacement)

	newMsgs, err := json.Marshal(msgs)
	if err != nil {
		return body, nil, nil
	}
	doc["messages"] = newMsgs
	out, err := json.Marshal(doc)
	if err != nil {
		return body, nil, nil
	}
	return out, savings, nil
}

// commandFromArgs pulls a shell command out of an OpenAI function-call
// arguments string (itself JSON). It looks for a "command" field regardless
// of the tool's name, so it works with whatever an agent calls its bash tool.
func commandFromArgs(arguments string) string {
	if arguments == "" {
		return ""
	}
	var probe struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(arguments), &probe); err != nil {
		return ""
	}
	return probe.Command
}

// contentText extracts text from a message's content, which is either a bare
// string or an array of {type:"text", text:"..."} parts.
func contentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		out := ""
		for _, p := range parts {
			if p.Type == "text" {
				out += p.Text
			}
		}
		return out
	}
	return ""
}

func str(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

func mustMarshalString(s string) json.RawMessage {
	out, err := json.Marshal(s)
	if err != nil {
		return json.RawMessage(`""`)
	}
	return out
}
