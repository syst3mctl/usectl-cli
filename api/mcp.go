package api

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

// ========== Agent tools (MCP) ==========
//
// The backend keeps one registry of agent-callable tools and exposes it as
// plain JSON at /api/mcp/tools. `usectl mcp serve` proxies that over stdio,
// which is why nothing MCP-specific lives in this client: it is an ordinary
// authenticated call and gets refresh-on-401 for free.

// MCPTool is one entry of the catalogue.
type MCPTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
	Annotations struct {
		ReadOnly    bool `json:"read_only"`
		Destructive bool `json:"destructive"`
		Idempotent  bool `json:"idempotent"`
	} `json:"annotations"`
}

// MCPToolResult is a tool's answer. IsError means the tool itself failed
// (bad arguments, access denied, no such machine) — the text is still what
// the agent should see.
type MCPToolResult struct {
	Content string `json:"content"`
	IsError bool   `json:"is_error"`
}

// ListMCPTools returns the tool catalogue; readOnly limits it to tools that
// never change state.
func (c *Client) ListMCPTools(readOnly bool) ([]MCPTool, error) {
	path := "/api/mcp/tools"
	if readOnly {
		path += "?read_only=1"
	}
	var out struct {
		Tools []MCPTool `json:"tools"`
	}
	if err := c.Get(path, &out); err != nil {
		return nil, err
	}
	return out.Tools, nil
}

// CallMCPTool runs one tool with raw JSON arguments.
func (c *Client) CallMCPTool(name string, args json.RawMessage) (*MCPToolResult, error) {
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	body := map[string]json.RawMessage{"arguments": args}
	var out MCPToolResult
	if err := c.Post(fmt.Sprintf("/api/mcp/tools/%s", url.PathEscape(name)), body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SetTimeout overrides the per-request HTTP timeout. Tool calls that search
// Loki/Tempo or probe a service legitimately take longer than the 30s the
// interactive commands use.
func (c *Client) SetTimeout(d time.Duration) {
	c.httpClient.Timeout = d
}
