package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/config"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to your account",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := api.NewClientUnauth(apiURL)

		var email, password string
		fmt.Print("Email: ")
		fmt.Scanln(&email)
		fmt.Print("Password: ")
		bytePw, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("read password: %w", err)
		}
		password = strings.TrimSpace(string(bytePw))
		fmt.Println()

		resp, err := client.Login(api.LoginRequest{Email: email, Password: password})
		if err != nil {
			return err
		}

		// Save token and API URL.
		cfg, _ := config.Load()
		cfg.Token = resp.Token
		if apiURL != "" {
			cfg.APIURL = apiURL
		}
		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}

		fmt.Printf("✓ Logged in as %s (%s)\n", resp.User.Username, resp.User.Email)
		return nil
	},
}

var registerCmd = &cobra.Command{
	Use:   "register",
	Short: "Create a new account",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := api.NewClientUnauth(apiURL)

		var email, username, password string
		fmt.Print("Email: ")
		fmt.Scanln(&email)
		fmt.Print("Username: ")
		fmt.Scanln(&username)
		fmt.Print("Password: ")
		bytePw, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("read password: %w", err)
		}
		password = strings.TrimSpace(string(bytePw))
		fmt.Println()

		resp, err := client.Register(api.RegisterRequest{Email: email, Username: username, Password: password})
		if err != nil {
			return err
		}

		cfg, _ := config.Load()
		cfg.Token = resp.Token
		if apiURL != "" {
			cfg.APIURL = apiURL
		}
		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}

		fmt.Printf("✓ Registered and logged in as %s (%s)\n", resp.User.Username, resp.User.Email)
		return nil
	},
}

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "View your profile",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}

		user, err := client.GetProfile()
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(user)
		}

		output.Table(
			[]string{"ID", "USERNAME", "EMAIL", "ROLE"},
			[][]string{{user.ID, user.Username, user.Email, user.Role}},
		)
		return nil
	},
}

var (
	profileUpdateUsername string
	profileUpdateEmail    string
	profileUpdatePassword string
)

var profileUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update your profile",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}

		req := api.UpdateProfileRequest{}
		if cmd.Flags().Changed("username") {
			req.Username = profileUpdateUsername
		}
		if cmd.Flags().Changed("email") {
			req.Email = profileUpdateEmail
		}
		if cmd.Flags().Changed("password") {
			req.Password = profileUpdatePassword
		}

		user, err := client.UpdateProfile(req)
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(user)
		}

		fmt.Printf("✓ Profile updated: %s (%s)\n", user.Username, user.Email)
		return nil
	},
}

func init() {
	profileUpdateCmd.Flags().StringVar(&profileUpdateUsername, "username", "", "New username")
	profileUpdateCmd.Flags().StringVar(&profileUpdateEmail, "email", "", "New email")
	profileUpdateCmd.Flags().StringVar(&profileUpdatePassword, "password", "", "New password")

	profileCmd.AddCommand(profileUpdateCmd)

	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(registerCmd)
	rootCmd.AddCommand(profileCmd)
}
