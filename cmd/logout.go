package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/config"
)

var logoutAll bool

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out and revoke this device's session",
	Long: `Log out of usectl.

Revokes this device's refresh token on the server and clears the local
credentials, so the saved session cannot be used again even if someone
copies ~/.usectl/config.json.

With --all, revokes every session on the account instead — use this if a
machine holding your credentials was lost or compromised.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		if cfg.Token == "" && cfg.RefreshToken == "" {
			fmt.Println("Not logged in.")
			return nil
		}

		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}

		if logoutAll {
			n, err := client.LogoutAll()
			if err != nil {
				return fmt.Errorf("sign out of all sessions: %w", err)
			}
			if err := clearCredentials(); err != nil {
				return err
			}
			fmt.Printf("✓ Signed out of %d session(s) across all devices\n", n)
			return nil
		}

		// Best-effort server-side revoke. Clearing the local credentials
		// still has to happen even if the API is unreachable — otherwise a
		// user who cannot reach the network cannot log out at all.
		if cfg.RefreshToken != "" {
			if err := client.Logout(cfg.RefreshToken); err != nil {
				fmt.Printf("warning: could not revoke session on the server: %v\n", err)
			}
		}
		if err := clearCredentials(); err != nil {
			return err
		}
		fmt.Println("✓ Logged out")
		return nil
	},
}

// clearCredentials wipes the tokens but preserves api_url, so the next login
// still targets the same instance.
func clearCredentials() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	cfg.Token = ""
	cfg.RefreshToken = ""
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

func init() {
	logoutCmd.Flags().BoolVar(&logoutAll, "all", false, "revoke every session on the account, not just this device")
	rootCmd.AddCommand(logoutCmd)
}
