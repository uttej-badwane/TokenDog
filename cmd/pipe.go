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

var pipeWebFetchCmd = &cobra.Command{
	Use:   "webfetch",
	Short: "Compress WebFetch PostToolUse responses",
	RunE:  runPipeWebFetch,
}

func init() {
	pipeCmd.AddCommand(pipeWebFetchCmd)
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

type postToolUseOutput struct {
	ToolResponse webFetchResponse `json:"tool_response"`
}

func runPipeWebFetch(_ *cobra.Command, _ []string) error {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}

	var input postToolUseInput
	if err := json.Unmarshal(data, &input); err != nil {
		return nil
	}

	// Parse tool_response as the WebFetch response object
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
		RawBytes:      resp.Bytes, // report actual page size, not just result size
		FilteredBytes: len(filtered),
		DurationMs:    elapsed,
	})

	if filtered == raw {
		return nil
	}

	resp.Result = filtered
	out, err := json.Marshal(postToolUseOutput{ToolResponse: resp})
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}
