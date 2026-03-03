package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/giorgi/usectl/config"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Model Context Protocol (MCP) integrations",
}

var mcpConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Print the Claude Desktop MCP configuration JSON",
	Long:  "Generates the JSON block required to connect Claude Desktop to your remote usectl cluster via Server-Sent Events (SSE).",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %v", err)
		}
		if cfg.Token == "" {
			return fmt.Errorf("not logged in. Please run 'usectl login' first")
		}

		apiURL := cfg.APIURL
		if apiURL == "" {
			apiURL = config.DefaultAPIURL
		}

		sseURL := fmt.Sprintf("%s/api/mcp/sse?token=%s", apiURL, cfg.Token)

		configBlock := map[string]interface{}{
			"mcpServers": map[string]interface{}{
				"usectl": map[string]interface{}{
					"command": "npx",
					"args": []string{
						"-y",
						"mcp-remote",
						sseURL,
					},
				},
			},
		}

		// Note: The above assumes a generic SSE to STDIO bridge like @modelcontextprotocol/client-sse exists or will exist soon,
		// or that the user can adapt the output. A more robust way is for 'usectl mcp serve' to bridge stdio -> sse locally.

		// For the sake of the plan let's print a config that connects directly using an HTTP/SSE bridge concept
		data, err := json.MarshalIndent(configBlock, "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(data))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(mcpCmd)
	mcpCmd.AddCommand(mcpConfigCmd)
}
