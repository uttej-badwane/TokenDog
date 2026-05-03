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

type postToolUseInput struct {
	SessionID      string         `json:"session_id"`
	TranscriptPath string         `json:"transcript_path"`
	ToolName       string         `json:"tool_name"`
	ToolInput      map[string]any `json:"tool_input"`
	ToolResponse   string         `json:"tool_response"`
}

type postToolUseOutput struct {
	ToolResponse string `json:"tool_response"`
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

	if input.ToolResponse == "" {
		return nil
	}

	start := time.Now()
	raw := input.ToolResponse
	filtered := filter.WebFetch(raw)
	elapsed := time.Since(start).Milliseconds()

	_ = analytics.Save(analytics.Record{
		Command:       "td pipe webfetch",
		Timestamp:     time.Now(),
		RawBytes:      len(raw),
		FilteredBytes: len(filtered),
		DurationMs:    elapsed,
	})

	if filtered == raw {
		return nil
	}

	out, err := json.Marshal(postToolUseOutput{ToolResponse: filtered})
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}
