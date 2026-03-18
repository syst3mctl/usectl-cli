package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	apiURL     string
	jsonOutput bool
)

// Version is set at build time by GoReleaser via ldflags.
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:     "usectl",
	Short:   "usectl — CLI for the usectl.com self-hosted deployment platform",
	Version: Version,
	Long: `usectl is the CLI for the usectl.com platform — a self-hosted Vercel alternative
running on K3s. It provides full lifecycle management for your applications.

Command Groups:
  login/register  Authenticate with the platform
  profile         View and update your user profile
  projects        Create, deploy, update, delete, and monitor projects
  envs            Manage custom environment variables for a project
  cron            Manage scheduled cron jobs for a project
  domains         Manage custom domains
  billing         Manage your subscription and payment
  github          GitHub App integration (OAuth, repos, branches)

All commands support --json for machine-readable output, making the CLI
suitable for scripting and AI agent automation.

Quick Start:
  1. usectl login                                    # Authenticate
  2. usectl github login                             # Connect GitHub
  3. usectl projects create --name my-app \           # Create project
     --repo https://github.com/user/repo \  
     --domain my-app --port 3000
  4. usectl projects deploy <id>                     # Deploy latest commit
  5. usectl projects logs <id>                       # View logs`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&apiURL, "api-url", "", "API base URL (default: from config or https://manager.usectl.com)")
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
}
