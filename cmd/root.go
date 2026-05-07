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

// Version is set at build time by GoReleaser via ldflags. The default below
// reflects the latest released version so unstamped local builds still report
// something meaningful.
var Version = "v1.1.0"

var rootCmd = &cobra.Command{
	Use:     "usectl",
	Short:   "usectl — CLI for the usectl.com self-hosted deployment platform",
	Version: Version,
	Long: `usectl is the CLI for the usectl.com platform — a self-hosted Vercel alternative
running on K3s. It provides full lifecycle management for your applications.

Command Groups:
  login/register   Authenticate with the platform
  profile          View and update your user profile
  projects         Create, deploy, update, delete, and monitor projects
  apps             Manage multi-app pods within a project (web/worker/release)
  addons           Manage project addons (database, redis, nats, mongodb, ...)
  envs             Manage custom env vars for a project
  vars             Build-time vs runtime variable exposure (per-app overrides)
  cron             Manage scheduled cron jobs (and run history)
  domains          Manage custom domains (incl. per-app pinning + verify)
  members          Manage project members, invitations, and per-developer scope
  quota            Inspect resource quota, recommendations, plan-resize previews
  notifications    View and acknowledge in-app notifications
  billing          Manage your subscription and payment
  project-billing  Per-project billing (status, portal)
  price            Server-side price calculator
  stack-detect     Detect language/framework for a (repo, ref)
  trial-status     Show your current trial countdown
  github           GitHub App integration (OAuth, repos, branches)
  orgs             Manage organizations, members, invitations
  admin            Admin-only user management

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
