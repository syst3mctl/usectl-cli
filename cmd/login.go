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

var loginPassword bool

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with the usectl platform",
	Long: `Log in and save the credentials to ~/.usectl/config.json.

By default this opens your browser, you approve the request in the dashboard,
and the CLI receives its token automatically — no password is typed into the
terminal and none is stored.

Use --password for the email/password prompt instead. That is the right choice
on a machine with no browser (a bare server, or SSH without port forwarding)
and in CI, where nothing can click Approve.`,
	Example: `  usectl login
  usectl login --password
  usectl login --api-url https://my-instance.com`,
	RunE: func(cmd *cobra.Command, args []string) error {
		base := apiURL
		if base == "" {
			if cfg, _ := config.Load(); cfg != nil {
				base = cfg.APIURL
			}
		}
		if base == "" {
			base = config.DefaultAPIURL
		}

		// The browser flow needs a browser AND someone to click Approve, so
		// fall back automatically when stdin is not a terminal rather than
		// hanging for five minutes in a pipeline.
		useWeb := !loginPassword
		if useWeb && !term.IsTerminal(int(os.Stdin.Fd())) {
			fmt.Fprintln(os.Stderr, "Not a terminal — falling back to password login.")
			useWeb = false
		}

		if useWeb {
			resp, err := runWebLogin(base)
			if err != nil {
				return err
			}
			return saveLogin(resp, apiURL)
		}

		client := api.NewClientUnauth(base)
		var email string
		fmt.Print("Email: ")
		fmt.Scanln(&email)
		fmt.Print("Password: ")
		bytePw, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("read password: %w", err)
		}
		fmt.Println()

		resp, err := client.Login(api.LoginRequest{
			Email:    email,
			Password: strings.TrimSpace(string(bytePw)),
		})
		if err != nil {
			return err
		}
		return saveLogin(resp, apiURL)
	},
}

var registerCmd = &cobra.Command{
	Use:   "register",
	Short: "Create a new account on the platform",
	Long: `Register a new user account. After registration, the account may need
admin approval before you can log in (depends on platform configuration).`,
	Example: `  usectl register`,
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
		cfg.RefreshToken = resp.RefreshToken
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
	Short: "View your user profile (ID, username, email, role)",
	Example: `  usectl profile
  usectl profile --json`,
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
	Short: "Update your username, email, or password",
	Long:  `Update one or more profile fields. Only the flags you provide will be changed.`,
	Example: `  usectl profile update --username newname
  usectl profile update --email new@example.com --password newpass123`,
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

	loginCmd.Flags().BoolVar(&loginPassword, "password", false, "Sign in with email and password instead of the browser")
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(registerCmd)
	rootCmd.AddCommand(profileCmd)
}
