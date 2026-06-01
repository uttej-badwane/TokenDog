package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"tokendog/internal/analytics"
	"tokendog/internal/compress"
	"tokendog/internal/filter"
	"tokendog/internal/hook"
	"tokendog/internal/stash"
)

// previewHeadLines / previewTailLines bound the head/tail kept when a large
// output is stashed. Tuned so the model keeps the orienting top of a log or
// file plus the trailing summary/errors most outputs end with.
const (
	previewHeadLines = 20
	previewTailLines = 5
)

// applyReversible stashes content and returns a compact preview when
// reversible mode is on and the content is large enough to be worth it.
// Returns (preview, true) on success, or ("", false) when the feature is
// off, the content is too small, eliding wouldn't shrink it, or the stash
// write fails — in every false case the caller keeps the lossless path.
func applyReversible(command, content string) (string, bool) {
	if !stash.Enabled() || len(content) < stash.MinSize() {
		return "", false
	}
	id, err := stash.Put(command, content)
	if err != nil {
		// Couldn't persist the original — never elide what we can't recover.
		return "", false
	}
	preview := stash.Preview(id, content, previewHeadLines, previewTailLines)
	if len(preview) >= len(content) {
		return "", false
	}
	return preview, true
}

// FilterHandler is the production RequestHandler. It parses an Anthropic
// Messages API request, finds tool_result content blocks in the LAST
// message only (cache safety — touching older content invalidates the
// prompt cache), looks up the corresponding tool_use to learn what
// command produced the output, runs the appropriate TD filter, and
// re-serializes.
//
// Why "last message only": Anthropic's prompt cache uses content-based
// hashing. Modifying historical tool_results would invalidate cache
// entries the model already paid to create — net cost goes UP. The last
// message contains the new content not yet seen by the API; modifying
// it costs nothing in cache invalidation while still saving on the
// upcoming charge.
func FilterHandler(req *http.Request, body []byte) ([]byte, error) {
	if len(body) == 0 || req.URL.Path != "/v1/messages" {
		return body, nil
	}
	var doc messagesRequest
	if err := json.Unmarshal(body, &doc); err != nil {
		// Malformed input — never modify what we couldn't parse.
		return body, nil
	}
	if len(doc.Messages) == 0 {
		return body, nil
	}

	// Build a tool_use_id → command lookup from ALL messages. The
	// matching tool_use is in an earlier (assistant) message; the
	// tool_result is in the (user) message we're about to filter.
	useByID := map[string]string{}
	for _, m := range doc.Messages {
		blocks, _ := unmarshalContent(m.Content)
		for _, b := range blocks {
			if b.Type == "tool_use" && b.ID != "" {
				cmd := extractCommand(b.Input)
				if cmd != "" {
					useByID[b.ID] = cmd
				}
			}
		}
	}

	// Compress tool descriptions. Tool definitions can contain verbose prose
	// (sometimes 500-2000 tokens). We strip filler/articles/pleasantries
	// using the same compressor as `td compress`, saving input tokens on
	// every API call.
	//
	// Cache safety: Claude Code occasionally marks the last tool with
	// cache_control to cache the tools array. We skip compression when any
	// tool carries cache_control so we never invalidate a warm cache entry.
	toolsModified := compressToolDescriptions(&doc)

	// Index every tool_result from earlier messages so we can replace a
	// duplicate in the last message with a back-reference. priorOrdinal is
	// the running tool-result count we continue from when numbering the last
	// message's own blocks.
	priorByHash, priorOrdinal := buildPriorResults(doc.Messages[:len(doc.Messages)-1], useByID)

	last := &doc.Messages[len(doc.Messages)-1]
	blocks, contentOK := unmarshalContent(last.Content)
	modified := false
	if contentOK {
		ordinal := priorOrdinal
		for i := range blocks {
			b := &blocks[i]
			if b.Type != "tool_result" || b.ToolUseID == "" {
				continue
			}
			raw := extractText(b.Content)
			if raw == "" {
				continue
			}
			ordinal++
			cmd := useByID[b.ToolUseID] // "" for non-Bash tools

			// Dedup pass: if this exact output already appears earlier in the
			// conversation, replace it with a back-reference. Lossless (the
			// full copy is verbatim above) and works for any tool_result,
			// including ones with no per-tool filter. Highest-value single
			// substitution, so it runs first.
			if deduped, ok := applyDedup(raw, priorByHash, ordinal); ok {
				b.Content = json.RawMessage(mustMarshalString(deduped))
				modified = true
				recordProxySaving(dedupLabel(cmd), raw, deduped)
				continue
			}

			// Lossless + reversible passes need a recognized Bash command;
			// dedup above already handled the command-agnostic case.
			if cmd == "" {
				continue
			}

			// Lossless pass — only for binaries we have a filter for.
			// ParseBinary returns ok=false for unsupported binaries; those
			// still flow to the reversible pass below, which is where the
			// long-tail of unhandled commands gets its only compression.
			filtered := raw
			applied := false
			if bin, args, pok := hook.ParseBinary(cmd); pok {
				if f, a := filter.Apply(bin, args, raw); a {
					filtered, applied = f, a
				}
			}

			// Reversible pass (opt-in via TD_REVERSIBLE=1): if the result is
			// still large after the lossless pass (or had no lossless filter
			// at all), stash the full original and replace it with a compact
			// head/tail preview that points at the td_retrieve MCP tool. This
			// breaks the lossless ceiling — nothing is lost, only deferred to
			// an on-demand round-trip.
			if reversed, rok := applyReversible(cmd, filtered); rok {
				b.Content = json.RawMessage(mustMarshalString(reversed))
				modified = true
				recordProxySaving(cmd+" (reversible)", raw, reversed)
				continue
			}

			if !applied || filtered == raw {
				continue
			}
			// Replace the content block. Use the simple string form (Anthropic
			// accepts either string OR array of {type:text,text:...}).
			b.Content = json.RawMessage(mustMarshalString(filtered))
			modified = true
			recordProxySaving(cmd, raw, filtered)
		}
		if modified {
			last.Content = mustMarshalContentBlocks(blocks)
		}
	}

	if !modified && !toolsModified {
		return body, nil
	}

	out, err := json.Marshal(doc)
	if err != nil {
		// Re-marshalling failed — bail to original to avoid sending
		// garbage to Anthropic.
		return body, nil
	}
	return out, nil
}

// compressToolDescriptions compresses description fields in doc.Tools.
// Returns true if any description was changed. Skips the entire tools array
// if any tool carries cache_control (preserves the cached payload).
func compressToolDescriptions(doc *messagesRequest) bool {
	if len(doc.Tools) == 0 {
		return false
	}
	// Parse tools as a generic slice so we preserve unknown fields.
	var tools []map[string]json.RawMessage
	if err := json.Unmarshal(doc.Tools, &tools); err != nil {
		return false
	}

	// Cache-safety check: if any tool has cache_control, skip entirely.
	for _, t := range tools {
		if cc, ok := t["cache_control"]; ok && len(cc) > 0 && string(cc) != "null" {
			return false
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
		return false
	}
	newTools, err := json.Marshal(tools)
	if err != nil {
		return false
	}
	// Record tools savings in analytics.
	recordProxySaving("tools/descriptions", string(doc.Tools), string(newTools))
	doc.Tools = newTools
	return true
}

// recordProxySaving writes a proxy-mode analytics record. Same Record
// schema used by hook mode so `td gain` aggregates both transparently.
func recordProxySaving(command, raw, filtered string) {
	rec := analytics.NewRecord("proxy: "+command, raw, filtered, 0)
	if err := analytics.Save(rec); err != nil {
		fmt.Fprintf(os.Stderr, "[td proxy] analytics save failed: %v\n", err)
	}
}

// messagesRequest is just enough of the Anthropic Messages API request
// shape to extract what we need. Unknown fields pass through via
// json.RawMessage so re-marshal preserves them exactly.
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
	Extra         map[string]any  `json:"-"` // unused — Go's encoding/json drops unknown fields, which is fine
}

type messageEntry struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type contentBlock struct {
	Type      string          `json:"type"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	Text      string          `json:"text,omitempty"`
	IsError   *bool           `json:"is_error,omitempty"`
	// Cache_control is left as raw so we don't drop it on re-marshal.
	CacheControl json.RawMessage `json:"cache_control,omitempty"`
}

// unmarshalContent handles both shapes Anthropic uses for message content:
// a bare string ("hello") or an array of content blocks. Returns blocks
// + ok; ok=false means the content was a string and there's nothing to
// filter (model didn't emit tool_result blocks for this message).
func unmarshalContent(raw json.RawMessage) ([]contentBlock, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	if raw[0] != '[' {
		return nil, false
	}
	var blocks []contentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, false
	}
	return blocks, true
}

// extractCommand pulls a "command" field out of a tool_use's input. For
// Bash tool_use the input is `{"command": "git status"}`. For other tools
// (Read, Edit, etc.) we'd need different keys; we return "" and skip
// non-Bash tools entirely.
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

// extractText pulls text content out of a tool_result block. Anthropic
// allows either a bare string or an array of {type:"text", text:"..."}
// content sub-blocks.
func extractText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// Bare string?
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	// Array of text blocks?
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
		// json.Marshal on a string never fails in practice; the only way
		// is invalid UTF-8, which we can sanitize by tolerating empty.
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
