package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"tokendog/internal/analytics"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run as a Model Context Protocol server (stdio JSON-RPC)",
	Long: `Exposes TokenDog's analytics over MCP so Claude Desktop, Cursor, or other
MCP-aware clients can query "how much have I saved this week?" in chat.

Add to ~/.claude/desktop_config.json (Claude Desktop) or your client's
equivalent:

  {
    "mcpServers": {
      "tokendog": {
        "command": "td",
        "args": ["mcp"]
      }
    }
  }

The server is read-only and runs entirely on stdio — no network sockets,
no privilege escalation. It exposes the same data ` + "`td gain --json`" + `
emits, formatted as MCP tool results.`,
	RunE: runMCP,
}

// MCP JSON-RPC 2.0 over stdio. Implements just enough of the protocol to
// answer Claude Desktop's tool-use queries: initialize handshake, tools/
// list, and tools/call. We deliberately don't pull in an MCP SDK — the
// protocol is small enough that a few hundred lines of stdlib JSON-RPC is
// cleaner than another dependency on a young, fast-moving spec.

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type mcpToolResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

func runMCP(_ *cobra.Command, _ []string) error {
	r := bufio.NewReader(os.Stdin)
	enc := json.NewEncoder(os.Stdout)

	for {
		// MCP frames each message as one JSON object per line over stdio.
		line, err := r.ReadBytes('\n')
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if len(line) <= 1 {
			continue
		}

		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			// Malformed input — emit a parse error and continue. Per JSON-RPC
			// spec the id is null when we couldn't even parse it.
			_ = enc.Encode(rpcResponse{
				JSONRPC: "2.0",
				Error:   &rpcError{Code: -32700, Message: "parse error"},
			})
			continue
		}

		resp := dispatchMCP(req)
		if resp.JSONRPC == "" {
			resp.JSONRPC = "2.0"
		}
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
}

func dispatchMCP(req rpcRequest) rpcResponse {
	switch req.Method {
	case "initialize":
		return rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities": map[string]any{
					"tools": map[string]any{},
				},
				"serverInfo": map[string]any{
					"name":    "tokendog",
					"version": Version,
				},
			},
		}
	case "tools/list":
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"tools": mcpTools()}}
	case "tools/call":
		return handleToolCall(req)
	case "notifications/initialized", "notifications/cancelled":
		// Notifications don't expect responses but we got the ID so we'll
		// emit one anyway. Most clients ignore extra responses.
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}
	default:
		return rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32601, Message: "method not found: " + req.Method},
		}
	}
}

func mcpTools() []mcpToolDef {
	return []mcpToolDef{
		{
			Name: "td_gain_summary",
			Description: "Get TokenDog's lifetime savings summary including total tokens saved, " +
				"USD saved at per-model rates, and per-model breakdown. Use this when the user " +
				"asks 'how much has TokenDog saved me?' or similar.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name: "td_gain_daily",
			Description: "Get TokenDog's daily savings breakdown over a time range. Returns one " +
				"row per day with token and USD savings. Use this for 'how much did td save me " +
				"this week?' style questions.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"days": map[string]any{
						"type":        "integer",
						"description": "Number of days back to include (default: 7, max: 365)",
						"default":     7,
					},
				},
			},
		},
		{
			Name:        "td_gain_session",
			Description: "Get TokenDog's savings for a specific Claude session, plus the actual Anthropic-reported token consumption for that session from the transcript.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_id": map[string]any{
						"type":        "string",
						"description": "Claude session id (UUID format)",
					},
				},
				"required": []string{"session_id"},
			},
		},
	}
}

type toolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

func handleToolCall(req rpcRequest) rpcResponse {
	var params toolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return rpcErr(req.ID, -32602, "invalid params: "+err.Error())
	}

	records, err := analytics.LoadAll()
	if err != nil {
		return mcpToolErr(req.ID, "failed to load analytics: "+err.Error())
	}
	analytics.ResolveModels(records)

	switch params.Name {
	case "td_gain_summary":
		summary, _ := analytics.Summarize(records)
		return mcpToolOK(req.ID, summary)

	case "td_gain_daily":
		days := 7
		if v, ok := params.Arguments["days"].(float64); ok {
			days = int(v)
			if days < 1 {
				days = 1
			} else if days > 365 {
				days = 365
			}
		}
		// Reuse the existing time-series builder for consistency.
		series := analytics.TimeSeriesData(records, false /* monthly */, false /* byModel */)
		// Trim to last `days` periods.
		if len(series) > days {
			series = series[len(series)-days:]
		}
		return mcpToolOK(req.ID, series)

	case "td_gain_session":
		sid, _ := params.Arguments["session_id"].(string)
		if sid == "" {
			return mcpToolErr(req.ID, "session_id is required")
		}
		var sessionRecords []analytics.Record
		for _, r := range records {
			if r.SessionID == sid {
				sessionRecords = append(sessionRecords, r)
			}
		}
		if len(sessionRecords) == 0 {
			return mcpToolErr(req.ID, "no records found for session "+sid)
		}
		summary, _ := analytics.Summarize(sessionRecords)
		return mcpToolOK(req.ID, summary)

	default:
		return mcpToolErr(req.ID, "unknown tool: "+params.Name)
	}
}

// mcpToolOK packages a JSON-serializable result as an MCP tool result.
// We always return a single text content block containing the JSON — most
// clients (including Claude Desktop) display it as code which is what we
// want for structured analytics data.
func mcpToolOK(id json.RawMessage, payload any) rpcResponse {
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return mcpToolErr(id, "failed to marshal result: "+err.Error())
	}
	return rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: string(body)}},
		},
	}
}

func mcpToolErr(id json.RawMessage, msg string) rpcResponse {
	return rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: mcpToolResult{
			IsError: true,
			Content: []mcpContent{{Type: "text", Text: msg}},
		},
	}
}

func rpcErr(id json.RawMessage, code int, msg string) rpcResponse {
	return rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcError{Code: code, Message: msg},
	}
}

// Silence the import lint when the package isn't otherwise used here —
// fmt is referenced indirectly via cobra usage strings. Belt-and-braces.
var _ = fmt.Sprintf
