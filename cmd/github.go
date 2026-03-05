package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"time"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/config"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

var githubCmd = &cobra.Command{
	Use:     "github",
	Aliases: []string{"gh"},
	Short:   "GitHub App integration",
}

var githubAppInfoCmd = &cobra.Command{
	Use:   "app-info",
	Short: "Show the GitHub App client ID",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		info, err := client.GetGitHubAppInfo()
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(info)
		}

		fmt.Printf("GitHub App Client ID: %s\n", info.ClientID)
		return nil
	},
}

var githubLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with GitHub via OAuth",
	Long:  "Opens your browser to authorize with GitHub, then automatically saves the token locally.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}

		// 1. Get the GitHub App client ID.
		info, err := client.GetGitHubAppInfo()
		if err != nil {
			return fmt.Errorf("get GitHub App info: %w", err)
		}

		// 2. Start a temporary local HTTP server on a fixed port.
		//    Add http://127.0.0.1:17249/callback as a Callback URL in your GitHub App settings.
		const callbackPort = 17249
		listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", callbackPort))
		if err != nil {
			return fmt.Errorf("port %d is busy — close whatever is using it and retry: %w", callbackPort, err)
		}
		redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", callbackPort)

		codeCh := make(chan string, 1)
		errCh := make(chan error, 1)

		mux := http.NewServeMux()
		mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
			code := r.URL.Query().Get("code")
			if code == "" {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, "<h1>Error</h1><p>Missing code parameter.</p>")
				errCh <- fmt.Errorf("GitHub did not return a code")
				return
			}
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, "<h1>✓ Authenticated!</h1><p>You can close this tab and return to the terminal.</p>")
			codeCh <- code
		})

		srv := &http.Server{Handler: mux}
		go func() {
			if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
				errCh <- err
			}
		}()

		// 3. Open the browser.
		authURL := fmt.Sprintf(
			"https://github.com/login/oauth/authorize?client_id=%s&redirect_uri=%s",
			info.ClientID, redirectURI,
		)

		fmt.Println("Opening browser for GitHub authorization...")
		if err := openBrowser(authURL); err != nil {
			fmt.Printf("Could not open browser. Please visit this URL manually:\n\n  %s\n\n", authURL)
		}
		fmt.Println("Waiting for authorization...")

		// 4. Wait for the callback (with 2-minute timeout).
		var code string
		select {
		case code = <-codeCh:
		case err := <-errCh:
			srv.Shutdown(context.Background())
			return err
		case <-time.After(2 * time.Minute):
			srv.Shutdown(context.Background())
			return fmt.Errorf("timed out waiting for GitHub authorization")
		}

		srv.Shutdown(context.Background())

		// 5. Exchange the code for a token.
		token, err := client.ExchangeGitHubCode(code)
		if err != nil {
			return fmt.Errorf("exchange code: %w", err)
		}

		// 6. Save to config.
		cfg, _ := config.Load()
		cfg.GitHubToken = token
		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}

		fmt.Println("✓ GitHub token saved. You can now use GitHub App commands.")
		return nil
	},
}

// openBrowser opens a URL in the default browser.
func openBrowser(url string) error {
	switch runtime.GOOS {
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return fmt.Errorf("unsupported platform")
	}
}

var githubTokenFlag string

func getGitHubToken() (string, error) {
	if githubTokenFlag != "" {
		return githubTokenFlag, nil
	}
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	if cfg.GitHubToken != "" {
		return cfg.GitHubToken, nil
	}
	return "", fmt.Errorf("no GitHub token found. Run 'usectl github login' or pass --github-token")
}

var githubInstallationsCmd = &cobra.Command{
	Use:     "installations",
	Aliases: []string{"installs"},
	Short:   "List GitHub App installations",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		ghToken, err := getGitHubToken()
		if err != nil {
			return err
		}

		installations, err := client.ListGitHubInstallations(ghToken)
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(installations)
		}

		rows := make([][]string, len(installations))
		for i, inst := range installations {
			rows[i] = []string{
				strconv.FormatInt(inst.ID, 10),
				inst.Account.Login,
				inst.TargetType,
			}
		}
		output.Table([]string{"ID", "ACCOUNT", "TYPE"}, rows)
		return nil
	},
}

var githubReposInstallID int64

var githubReposCmd = &cobra.Command{
	Use:   "repos",
	Short: "List repositories for a GitHub App installation",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		ghToken, err := getGitHubToken()
		if err != nil {
			return err
		}

		repos, err := client.ListGitHubRepos(ghToken, githubReposInstallID)
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(repos)
		}

		rows := make([][]string, len(repos))
		for i, r := range repos {
			visibility := "public"
			if r.Private {
				visibility = "private"
			}
			rows[i] = []string{r.FullName, visibility, r.CloneURL}
		}
		output.Table([]string{"REPO", "VISIBILITY", "CLONE URL"}, rows)
		return nil
	},
}

var (
	githubBranchesInstallID int64
	githubBranchesRepo      string
)

var githubBranchesCmd = &cobra.Command{
	Use:   "branches",
	Short: "List branches for a repository",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		ghToken, err := getGitHubToken()
		if err != nil {
			return err
		}

		branches, err := client.ListGitHubBranches(ghToken, githubBranchesInstallID, githubBranchesRepo)
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(branches)
		}

		rows := make([][]string, len(branches))
		for i, b := range branches {
			protected := "no"
			if b.Protected {
				protected = "yes"
			}
			rows[i] = []string{b.Name, protected}
		}
		output.Table([]string{"BRANCH", "PROTECTED"}, rows)
		return nil
	},
}

func init() {
	// Global GitHub token flag for all github subcommands
	githubCmd.PersistentFlags().StringVar(&githubTokenFlag, "github-token", "", "GitHub user token (default: from config)")

	// Repos flags
	githubReposCmd.Flags().Int64Var(&githubReposInstallID, "installation", 0, "Installation ID (required)")
	githubReposCmd.MarkFlagRequired("installation")

	// Branches flags
	githubBranchesCmd.Flags().Int64Var(&githubBranchesInstallID, "installation", 0, "Installation ID (required)")
	githubBranchesCmd.Flags().StringVar(&githubBranchesRepo, "repo", "", "Repository in owner/name format (required)")
	githubBranchesCmd.MarkFlagRequired("installation")
	githubBranchesCmd.MarkFlagRequired("repo")

	githubCmd.AddCommand(githubAppInfoCmd)
	githubCmd.AddCommand(githubLoginCmd)
	githubCmd.AddCommand(githubInstallationsCmd)
	githubCmd.AddCommand(githubReposCmd)
	githubCmd.AddCommand(githubBranchesCmd)

	rootCmd.AddCommand(githubCmd)
}
