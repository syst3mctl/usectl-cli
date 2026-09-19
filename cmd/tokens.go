package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

// Scoped agent API keys (mig 084). A key acts as you, narrowed: to some
// machines, read-only or write, optionally to admin areas, with an expiry.
// It replaces the 24h session token in MCP configs and CI:
//
//	usectl tokens create --name claude --machine api,staging --access read --ttl 30d
//	usectl mcp config --token usectl_agt_…        # MCP block that never expires daily
//	USECTL_TOKEN=usectl_agt_… usectl machines list  # any command under the key
//
// The plaintext is printed once. Keys never mint keys and never change
// account settings, whatever their access.

var (
	tokenName     string
	tokenMachines []string
	tokenAccess   string
	tokenAdmin    []string
	tokenTTL      string
	tokenAll      bool
)

var tokensCmd = &cobra.Command{
	Use:   "tokens",
	Short: "Scoped agent API keys for MCP clients and CI",
	Long: `Long-lived, revocable keys narrowed to machines, read-only or write, and
(for admins) named admin areas. Use them where a 24h session is wrong: MCP
client configs, CI jobs, scripts on servers.

  usectl tokens create --name claude-code --machine api --access read --ttl 30d
  usectl tokens create --name ci-sync --access write --admin knowledge --ttl never
  usectl tokens list
  usectl tokens revoke usectl_agt_3fa2

A read key can only GET and only run read-only tools. A machine-scoped key
cannot see other machines at all (404, not 403). Every key is blocked from
account, user, organisation and key management.`,
}

var tokensCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a key (printed once)",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if strings.TrimSpace(tokenName) == "" {
			return fmt.Errorf("--name is required")
		}
		req := api.CreateAgentTokenRequest{Name: tokenName, Access: tokenAccess, AdminScopes: tokenAdmin}
		for _, m := range tokenMachines {
			for _, one := range strings.Split(m, ",") {
				one = strings.TrimSpace(one)
				if one == "" {
					continue
				}
				id, err := resolveMachine(client, one)
				if err != nil {
					return err
				}
				req.ProjectIDs = append(req.ProjectIDs, id)
			}
		}
		if tokenTTL != "" {
			d, err := parseTTLDays(tokenTTL)
			if err != nil {
				return err
			}
			req.TTLDays = &d
		}
		resp, err := client.CreateAgentToken(req)
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(resp)
		}
		k := resp.Key
		fmt.Printf("✓ Key %q created (%s)\n\n  %s\n\n", k.Name, k.Prefix, resp.Token)
		fmt.Println("This is the only time the key is shown. Store it now.")
		fmt.Printf("  Scope    %s · %s%s\n", scopeText(k), k.Access, adminText(k))
		fmt.Printf("  Expires  %s\n", expiryText(k))
		fmt.Printf("\nUse it:\n  usectl mcp config --token %s\n  USECTL_TOKEN=%s usectl machines list\n", resp.Token, resp.Token)
		return nil
	},
}

var tokensListCmd = &cobra.Command{
	Use:   "list",
	Short: "List your keys (admins: --all for everyone's)",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		var keys []api.AgentToken
		if tokenAll {
			keys, err = client.AdminListAgentTokens()
		} else {
			keys, _, err = client.ListAgentTokens()
		}
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(keys)
		}
		if len(keys) == 0 {
			fmt.Println("No keys. Create one with 'usectl tokens create --name <name>'.")
			return nil
		}
		rows := make([][]string, 0, len(keys))
		for _, k := range keys {
			state := "live"
			if k.RevokedAt != nil {
				state = "revoked"
			}
			last := "never"
			if k.LastUsedAt != nil {
				last = humanAge(*k.LastUsedAt)
			}
			row := []string{k.Prefix, k.Name, k.Access + adminText(k), scopeText(k), expiryText(k), last, state}
			if tokenAll {
				row = append([]string{k.OwnerEmail}, row...)
			}
			rows = append(rows, row)
		}
		hdr := []string{"KEY", "NAME", "ACCESS", "MACHINES", "EXPIRES", "LAST USED", "STATE"}
		if tokenAll {
			hdr = append([]string{"OWNER"}, hdr...)
		}
		output.Table(hdr, rows)
		return nil
	},
}

var tokensRevokeCmd = &cobra.Command{
	Use:   "revoke <id-or-prefix>",
	Short: "Revoke a key immediately",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		var keys []api.AgentToken
		if tokenAll {
			keys, err = client.AdminListAgentTokens()
		} else {
			keys, _, err = client.ListAgentTokens()
		}
		if err != nil {
			return err
		}
		id := ""
		for _, k := range keys {
			if k.ID == args[0] || k.Prefix == args[0] || strings.HasPrefix(k.Prefix, args[0]) && len(args[0]) >= 8 || k.Name == args[0] {
				if id != "" && id != k.ID {
					return fmt.Errorf("%q matches more than one key — use the id", args[0])
				}
				id = k.ID
			}
		}
		if id == "" {
			return fmt.Errorf("no key matches %q", args[0])
		}
		if tokenAll {
			err = client.AdminRevokeAgentToken(id)
		} else {
			err = client.RevokeAgentToken(id)
		}
		if err != nil {
			return err
		}
		fmt.Println("✓ Key revoked. Clients using it get 401 \"agent key revoked\" from now on.")
		return nil
	},
}

func scopeText(k api.AgentToken) string {
	if len(k.ProjectIDs) == 0 {
		return "all machines"
	}
	return fmt.Sprintf("%d machine(s)", len(k.ProjectIDs))
}
func adminText(k api.AgentToken) string {
	if len(k.AdminScopes) == 0 {
		return ""
	}
	return " · admin:" + strings.Join(k.AdminScopes, ",")
}
func expiryText(k api.AgentToken) string {
	if k.ExpiresAt == nil {
		return "never"
	}
	return "in " + strings.TrimSuffix(humanAge(*k.ExpiresAt), " ago")
}

// parseTTLDays accepts 30, 30d, 12h (rounded up to a day), never.
func parseTTLDays(s string) (int, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "never" || s == "0" {
		return 0, nil
	}
	if strings.HasSuffix(s, "d") {
		s = strings.TrimSuffix(s, "d")
	}
	if strings.HasSuffix(s, "h") {
		h, err := strconv.Atoi(strings.TrimSuffix(s, "h"))
		if err != nil {
			return 0, fmt.Errorf("--ttl must be like 30d, 12h or never")
		}
		return (h + 23) / 24, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("--ttl must be like 30d, 12h or never")
	}
	return n, nil
}

func init() {
	tokensCreateCmd.Flags().StringVar(&tokenName, "name", "", "What the key is for (required)")
	tokensCreateCmd.Flags().StringSliceVar(&tokenMachines, "machine", nil, "Restrict to these machines (names or ids, repeatable/comma-separated); default all")
	tokensCreateCmd.Flags().StringVar(&tokenAccess, "access", "read", "read (GET + read-only tools) or write")
	tokensCreateCmd.Flags().StringSliceVar(&tokenAdmin, "admin", nil, "Admin areas the key may use (admins only): all, knowledge, digest, tokens, users, billing, …")
	tokensCreateCmd.Flags().StringVar(&tokenTTL, "ttl", "", "Lifetime: 30d, 12h, never (default 90d; never is admin-only)")
	tokensListCmd.Flags().BoolVar(&tokenAll, "all", false, "Every user's keys (admin)")
	tokensRevokeCmd.Flags().BoolVar(&tokenAll, "all", false, "Revoke another user's key (admin)")
	tokensCmd.AddCommand(tokensCreateCmd, tokensListCmd, tokensRevokeCmd)
	rootCmd.AddCommand(tokensCmd)
}
