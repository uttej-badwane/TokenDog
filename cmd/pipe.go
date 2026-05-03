package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
	"tokendog/internal/analytics"
	"tokendog/internal/filter"
)

var pipeCmd = &cobra.Command{
	Use:   "pipe",
	Short: "Filter PostToolUse hook responses",
}

var (
	pipeWebFetchCmd = &cobra.Command{
		Use:   "webfetch",
		Short: "Compress WebFetch PostToolUse responses",
		RunE:  runPipeWebFetch,
	}
	pipeGlobCmd = &cobra.Command{
		Use:   "glob",
		Short: "Compress Glob PostToolUse responses",
		RunE:  runPipeGlob,
	}
	pipeGrepCmd = &cobra.Command{
		Use:   "grep",
		Short: "Compress Grep PostToolUse responses",
		RunE:  runPipeGrep,
	}
	pipeWebSearchCmd = &cobra.Command{
		Use:   "websearch",
		Short: "Compress WebSearch PostToolUse responses",
		RunE:  runPipeWebSearch,
	}
)

func init() {
	pipeCmd.AddCommand(pipeWebFetchCmd)
	pipeCmd.AddCommand(pipeGlobCmd)
	pipeCmd.AddCommand(pipeGrepCmd)
	pipeCmd.AddCommand(pipeWebSearchCmd)
}

// webFetchResponse matches Claude Code's WebFetch tool_response object
type webFetchResponse struct {
	Bytes    int    `json:"bytes"`
	Code     int    `json:"code"`
	CodeText string `json:"codeText"`
	Result   string `json:"result"`
	URL      string `json:"url"`
}

type postToolUseInput struct {
	SessionID      string          `json:"session_id"`
	TranscriptPath string          `json:"transcript_path"`
	ToolName       string          `json:"tool_name"`
	ToolInput      map[string]any  `json:"tool_input"`
	ToolResponse   json.RawMessage `json:"tool_response"`
}

type webFetchOutput struct {
	ToolResponse webFetchResponse `json:"tool_response"`
}

type stringOutput struct {
	ToolResponse string `json:"tool_response"`
}

func readPostToolUse() (*postToolUseInput, error) {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, err
	}
	var input postToolUseInput
	if err := json.Unmarshal(data, &input); err != nil {
		return nil, nil
	}
	return &input, nil
}

// extractContent attempts to extract textual content from tool_response,
// handling both raw-string and object-with-content-field shapes.
func extractContent(raw json.RawMessage) (content string, container map[string]any, isString bool) {
	if len(raw) == 0 {
		return "", nil, false
	}
	// Try as plain string
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil, true
	}
	// Try as object — look for common content fields
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return "", nil, false
	}
	for _, key := range []string{"result", "content", "output", "text", "matches"} {
		if v, ok := obj[key].(string); ok {
			return v, obj, false
		}
	}
	return "", obj, false
}

func writeStringResponse(content string) error {
	out, err := json.Marshal(stringOutput{ToolResponse: content})
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}

func writeObjectResponse(obj map[string]any, content string) error {
	for _, key := range []string{"result", "content", "output", "text", "matches"} {
		if _, ok := obj[key]; ok {
			obj[key] = content
			break
		}
	}
	out, err := json.Marshal(map[string]any{"tool_response": obj})
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}

func runPipeWebFetch(_ *cobra.Command, _ []string) error {
	input, err := readPostToolUse()
	if err != nil || input == nil {
		return err
	}

	var resp webFetchResponse
	if err := json.Unmarshal(input.ToolResponse, &resp); err != nil || resp.Result == "" {
		return nil
	}

	start := time.Now()
	raw := resp.Result
	filtered := filter.WebFetch(raw)
	elapsed := time.Since(start).Milliseconds()

	_ = analytics.Save(analytics.Record{
		Command:       "td pipe webfetch",
		Timestamp:     time.Now(),
		RawBytes:      resp.Bytes,
		FilteredBytes: len(filtered),
		DurationMs:    elapsed,
	})

	if filtered == raw {
		return nil
	}
	resp.Result = filtered
	out, err := json.Marshal(webFetchOutput{ToolResponse: resp})
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}

func runPipeGlob(_ *cobra.Command, _ []string) error {
	return runGenericPipe("glob", filter.Glob)
}

func runPipeGrep(_ *cobra.Command, _ []string) error {
	return runGenericPipe("grep", filter.Grep)
}

func runPipeWebSearch(_ *cobra.Command, _ []string) error {
	return runGenericPipe("websearch", filter.WebSearch)
}

func runGenericPipe(name string, filterFn func(string) string) error {
	input, err := readPostToolUse()
	if err != nil || input == nil {
		return err
	}

	content, container, isString := extractContent(input.ToolResponse)
	if content == "" {
		return nil
	}

	start := time.Now()
	filtered := filterFn(content)
	elapsed := time.Since(start).Milliseconds()

	_ = analytics.Save(analytics.Record{
		Command:       "td pipe " + name,
		Timestamp:     time.Now(),
		RawBytes:      len(content),
		FilteredBytes: len(filtered),
		DurationMs:    elapsed,
	})

	if filtered == content {
		return nil
	}

	if isString {
		return writeStringResponse(filtered)
	}
	return writeObjectResponse(container, filtered)
}
