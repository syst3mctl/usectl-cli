package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/config"
	"github.com/giorgi/usectl/output"
)

// usectl mcp — run usectl as a Model Context Protocol server so Claude Code,
// Cursor, Claude Desktop and any other MCP client can operate machines
// directly (deploy, rollback, logs, traces, envs, addons, crons, quota…).
//
// `serve` speaks MCP over stdio and proxies every tool call to the API's
// /api/mcp/tools endpoints with the credentials from `usectl login`. The
// tool catalogue is fetched from the server at start-up, so a new backend
// tool appears here without a CLI release, and the access token is refreshed
// transparently like any other command — no token ever lands in a client's
// config file and nothing has to speak SSE with a ?token= in the URL.

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run usectl as an MCP server for Claude Code, Cursor and Claude Desktop",
	Long: `Expose your machines to an AI assistant over the Model Context Protocol.

  usectl mcp serve            start the stdio server (what MCP clients launch)
  usectl mcp config           print the client config snippet
  usectl mcp tools            list the tools the server would expose

Quick start with Claude Code:

  claude mcp add usectl -- usectl mcp serve

Tool calls run as you, with your role on each machine. Add --read-only to
hide every tool that changes state.`,
}

var (
	mcpReadOnly bool
	mcpMachine  string
)

var mcpServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Serve MCP over stdio (launched by the MCP client, not by hand)",
	Args:  cobra.NoArgs,
	// stderr is the MCP client's log; a usage dump there is noise.
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// stdout is the protocol channel: everything human goes to stderr.
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if client.Token == "" {
			return fmt.Errorf("not logged in — run 'usectl login', or set USECTL_TOKEN to an agent key (usectl tokens create)")
		}
		client.SetTimeout(60 * time.Second)

		readOnly := mcpReadOnly || envTrue("USECTL_MCP_READ_ONLY")
		pinned := strings.TrimSpace(mcpMachine)
		if pinned == "" {
			pinned = strings.TrimSpace(os.Getenv("USECTL_MCP_MACHINE"))
		}
		if pinned != "" {
			// Resolve once so a typo fails at start-up, not on the first call.
			id, err := resolveMachine(client, pinned)
			if err != nil {
				return err
			}
			pinned = id
		}

		catalogue, err := client.ListMCPTools(readOnly)
		if err != nil {
			return fmt.Errorf("fetch tool catalogue: %w", err)
		}

		s := server.NewMCPServer("usectl", Version,
			server.WithToolCapabilities(false),
			server.WithInstructions(mcpInstructions(readOnly, pinned)),
		)
		for _, t := range catalogue {
			t := t
			schema := t.InputSchema
			if pinned != "" {
				schema = stripSchemaProperty(schema, "machine")
			}
			tool := mcp.NewToolWithRawSchema(t.Name, t.Description, schema)
			tool.Annotations = mcp.ToolAnnotation{
				Title:           t.Name,
				ReadOnlyHint:    mcp.ToBoolPtr(t.Annotations.ReadOnly),
				DestructiveHint: mcp.ToBoolPtr(t.Annotations.Destructive),
				IdempotentHint:  mcp.ToBoolPtr(t.Annotations.Idempotent),
				OpenWorldHint:   mcp.ToBoolPtr(false),
			}
			s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				args := req.GetArguments()
				if args == nil {
					args = map[string]any{}
				}
				if pinned != "" {
					args["machine"] = pinned
				}
				raw, _ := json.Marshal(args)
				res, err := client.CallMCPTool(t.Name, raw)
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				if res.IsError {
					return mcp.NewToolResultError(res.Content), nil
				}
				return mcp.NewToolResultText(res.Content), nil
			})
		}

		fmt.Fprintf(os.Stderr, "usectl mcp: serving %d tools over stdio (read-only=%v)\n", len(catalogue), readOnly)
		return server.ServeStdio(s)
	},
}

var mcpToolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "List the tools the MCP server exposes",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		catalogue, err := client.ListMCPTools(mcpReadOnly)
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(catalogue)
		}
		for _, t := range catalogue {
			kind := "write"
			switch {
			case t.Annotations.ReadOnly:
				kind = "read"
			case t.Annotations.Destructive:
				kind = "destructive"
			}
			fmt.Printf("%-22s %-12s %s\n", t.Name, kind, firstSentence(t.Description))
		}
		return nil
	},
}

var mcpConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Print the MCP client configuration for Claude Code, Cursor or Claude Desktop",
	Long: `Prints the JSON block that tells an MCP client to launch "usectl mcp serve".

  usectl mcp config                      # generic mcpServers block
  usectl mcp config --client claude-code # the one-line 'claude mcp add' command
  usectl mcp config --read-only          # server hides state-changing tools
  usectl mcp config --token usectl_agt_… # use a scoped agent key instead of the 24h session`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		mcpToken, _ := cmd.Flags().GetString("token")
		if mcpToken == "" && cfg.Token == "" {
			fmt.Fprintln(os.Stderr, "warning: not logged in — run 'usectl login' or pass --token (usectl tokens create) before the server can start")
		}
		if mcpToken != "" && !strings.HasPrefix(mcpToken, api.AgentTokenPrefix) {
			fmt.Fprintln(os.Stderr, "warning: --token is not an agent key (usectl_agt_…); a session token expires in 24h")
		}
		exe, _ := os.Executable()
		if exe == "" {
			exe = "usectl"
		}
		srvArgs := []string{"mcp", "serve"}
		if mcpReadOnly {
			srvArgs = append(srvArgs, "--read-only")
		}
		if mcpMachine != "" {
			srvArgs = append(srvArgs, "--machine", mcpMachine)
		}
		if apiURL != "" {
			srvArgs = append(srvArgs, "--api-url", apiURL)
		}

		client, _ := cmd.Flags().GetString("client")
		switch client {
		case "claude-code":
			if mcpToken != "" {
				fmt.Printf("claude mcp add usectl -e USECTL_TOKEN=%s -- %s %s\n", mcpToken, exe, strings.Join(srvArgs, " "))
			} else {
				fmt.Printf("claude mcp add usectl -- %s %s\n", exe, strings.Join(srvArgs, " "))
			}
			return nil
		case "claude-desktop", "cursor", "":
			server := map[string]any{"command": exe, "args": srvArgs}
			if mcpToken != "" {
				// The key rides in the environment, so the config file never
				// needs the session token and never expires with it.
				server["env"] = map[string]string{"USECTL_TOKEN": mcpToken}
			}
			block := map[string]any{
				"mcpServers": map[string]any{
					"usectl": server,
				},
			}
			data, _ := json.MarshalIndent(block, "", "  ")
			fmt.Println(string(data))
			switch client {
			case "claude-desktop":
				fmt.Fprintln(os.Stderr, "\nAdd to: ~/Library/Application Support/Claude/claude_desktop_config.json (macOS) or %APPDATA%\\Claude\\claude_desktop_config.json (Windows)")
			case "cursor":
				fmt.Fprintln(os.Stderr, "\nAdd to: ~/.cursor/mcp.json (global) or .cursor/mcp.json in the project")
			}
			return nil
		default:
			return fmt.Errorf("unknown --client %q (claude-code, claude-desktop, cursor)", client)
		}
	},
}

func mcpInstructions(readOnly bool, pinned string) string {
	var b strings.Builder
	b.WriteString("usectl is a platform that runs applications on Kubernetes. A 'machine' is a project; a 'pod' is a deployable app inside it; addons are databases, caches, queues and buckets. ")
	b.WriteString("Users have no kubectl or shell access — everything happens through these tools. ")
	if pinned != "" {
		b.WriteString("This server is pinned to one machine; the 'machine' argument is implied. ")
	} else {
		b.WriteString("Start with list_machines to learn names, then get_machine or get_diagnostics before acting. ")
	}
	if readOnly {
		b.WriteString("This server is read-only: it can inspect but not change anything.")
	} else {
		b.WriteString("Confirm with the user before deploy, rollback, delete_*, remove_addon or set_envs on production.")
	}
	return b.String()
}

func stripSchemaProperty(schema json.RawMessage, name string) json.RawMessage {
	var s map[string]any
	if json.Unmarshal(schema, &s) != nil {
		return schema
	}
	if props, ok := s["properties"].(map[string]any); ok {
		delete(props, name)
	}
	if req, ok := s["required"].([]any); ok {
		kept := req[:0]
		for _, r := range req {
			if r != name {
				kept = append(kept, r)
			}
		}
		if len(kept) == 0 {
			delete(s, "required")
		} else {
			s["required"] = kept
		}
	}
	out, err := json.Marshal(s)
	if err != nil {
		return schema
	}
	return out
}

func envTrue(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes":
		return true
	}
	return false
}

func firstSentence(s string) string {
	if i := strings.Index(s, ". "); i > 0 {
		return s[:i+1]
	}
	return s
}

func init() {
	rootCmd.AddCommand(mcpCmd)
	mcpCmd.AddCommand(mcpServeCmd, mcpToolsCmd, mcpConfigCmd)
	for _, c := range []*cobra.Command{mcpServeCmd, mcpToolsCmd, mcpConfigCmd} {
		c.Flags().BoolVar(&mcpReadOnly, "read-only", false, "expose only tools that never change state (also USECTL_MCP_READ_ONLY=1)")
	}
	mcpServeCmd.Flags().StringVar(&mcpMachine, "machine", "", "pin every tool to one machine (also USECTL_MCP_MACHINE)")
	mcpConfigCmd.Flags().StringVar(&mcpMachine, "machine", "", "pin the configured server to one machine")
	mcpConfigCmd.Flags().String("token", "", "Agent key (usectl_agt_…) to embed as USECTL_TOKEN instead of the 24h session")
	mcpConfigCmd.Flags().String("client", "", "target client: claude-code, claude-desktop, cursor")
}
