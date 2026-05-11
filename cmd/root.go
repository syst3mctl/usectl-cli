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
var Version = "v1.1.3"

var rootCmd = &cobra.Command{
	Use:     "usectl",
	Short:   "usectl — CLI for the usectl.com deployment platform",
	Version: Version,
	Long: `usectl is the CLI for the usectl.com platform. It provides full lifecycle
management for your applications on Kubernetes.

Command Groups:
  login/register   Authenticate with the platform
  profile          View and update your user profile
  machines         Create, deploy, update, delete, monitor (a.k.a. "projects")
  machines pods    List/inspect/shell/restart K8s pods inside a machine
  apps             Manage multi-app pods within a machine (web/worker/release)
  addons           Manage machine addons (database, redis, nats, mongodb, ...)
  envs             Manage custom env vars for a machine
  vars             Build-time vs runtime variable exposure (per-app overrides)
  cron             Manage scheduled cron jobs (and run history)
  domains          Manage custom domains (incl. per-app pinning + verify)
  members          Manage machine members, invitations, and per-developer scope
  quota            Inspect resource quota, recommendations, plan-resize previews
  notifications    View and acknowledge in-app notifications
  billing          Manage your subscription and payment
  project-billing  Per-machine billing (status, portal)
  price            Server-side price calculator
  stack-detect     Detect language/framework for a (repo, ref)
  trial-status     Show your current trial countdown
  github           GitHub App integration (OAuth, repos, branches)
  orgs             Manage organizations, members, invitations
  admin            Admin-only user management

All commands support --json for machine-readable output, making the CLI
suitable for scripting and AI agent automation.

The dashboard renamed "Project" → "Machine" (see PROMPT_rename_project_to_
machine_minimal.md). The CLI follows: 'usectl machines ...' is the new spelling.
'usectl projects ...' / 'usectl p ...' still work as aliases — no scripts break.

Quick Start:
  1. usectl login                                    # Authenticate
  2. usectl github login                             # Connect GitHub
  3. usectl machines create --name my-app \           # Create machine
     --repo https://github.com/user/repo \
     --domain my-app --port 3000
  4. usectl machines deploy <id>                     # Deploy latest commit
  5. usectl machines pods <id>                       # See running pods
  6. usectl machines logs <id> -f                    # Tail logs

──────────────────────────────────────────────────────────────────────────
AI AGENT GUIDE
──────────────────────────────────────────────────────────────────────────

Mental model:
  - A "machine" is one Kubernetes namespace (kdeploy-<name>) + one domain.
  - "Apps" are pods inside a machine, each built from its own repo/branch.
    Use multiple apps when frontend + backend + worker belong to the same
    product and should share addons. Use separate machines for unrelated
    products.
  - "Addons" (postgres, redis, nats, mongodb, s3, ...) are provisioned per
    machine and their credentials are auto-injected into every app pod as
    env vars (DATABASE_URL, REDIS_URL, ...). Do not set these by hand.

Do I need to commit a Dockerfile?
  Run 'usectl stack-detect --repo <url> --ref <branch>' first. Auto-build
  rules (no Dockerfile required, port is forced to 80):
    next.config.*    → Next.js (npm run build → next start -p 80)
    vite.config.*    → Vite/React/Vue (npm run build → nginx serves /dist)
    package.json     → Generic Node.js (npm start, port 80)
    only HTML/CSS/JS → Static site served by nginx
  Everything else (Go, Python, Rust, Java, custom Node, ...) MUST ship a
  Dockerfile in the repo root that EXPOSEs its port, and you must pass
  --port matching that EXPOSE.

Aliases (all equivalent):
  machines = machine = m = projects = project = p

Scripting:
  Pass --json to any command for structured output. Exit code is non-zero
  on failure. Auth token is read from ~/.usectl/config.json after 'login'.

Common recipes:
  # Next.js site, no Dockerfile, with Postgres:
  usectl machines create --name blog --repo https://github.com/me/blog \
    --domain blog --port 80 --db

  # Go API (Dockerfile in repo) on port 8080:
  usectl machines create --name api --repo https://github.com/me/api \
    --domain api --port 8080

  # Add a worker pod to an existing machine:
  usectl apps create <machine-id> --name worker \
    --repo https://github.com/me/api --kind worker --command "go run ./worker"`,
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
