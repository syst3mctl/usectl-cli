package cmd

import (
	"fmt"
	"strconv"

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
	Use:   "login <code>",
	Short: "Exchange a GitHub OAuth code for a token",
	Long:  "After authorizing via the GitHub App OAuth flow, exchange the code for a GitHub user token and save it locally.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}

		token, err := client.ExchangeGitHubCode(args[0])
		if err != nil {
			return err
		}

		// Save GitHub token to config.
		cfg, _ := config.Load()
		cfg.GitHubToken = token
		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}

		fmt.Println("✓ GitHub token saved. You can now use GitHub App commands.")
		return nil
	},
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
	return "", fmt.Errorf("no GitHub token found. Run 'usectl github login <code>' or pass --github-token")
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
