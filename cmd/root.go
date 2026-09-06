package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
)

var (
	apiURL     string
	jsonOutput bool
	colorMode  string
)

// Version is set at build time by GoReleaser via ldflags. The default below
// reflects the latest released version so unstamped local builds still report
// something meaningful.
var Version = "v1.1.4"

var rootCmd = &cobra.Command{
	Use:     "usectl",
	Short:   "usectl — CLI for the usectl.com deployment platform",
	Version: Version,
	Long: `usectl is the CLI for the usectl.com platform. It provides full lifecycle
management for your applications on Kubernetes.

Command Groups:
  login/register   Authenticate with the platform
  profile          View and update your user profile
  machines         Create, resize, delete and monitor machines
  machines pods    Every pod in a machine: config, limits, and where it runs
  machines addons  Addons (database, redis, nats, mongodb, s3, ...)
  machines members Members, roles, invitations, per-developer scope
  machines groups  Partition pods/addons into isolated group namespaces
  machines quota   Resource quota, usage against limits, resize previews
  machines envs    Machine-wide environment variables
  machines domains Custom domains (incl. per-pod pinning + verify)
  machines cron    Scheduled cron jobs and run history
  apps             Alias for 'machines pods' (multi-pod management)
  deployments      Deployment history; roll back to a previous build
  registry         Container-image allowance and what is consuming it
  notifications    View and acknowledge in-app notifications
  billing          Manage your subscription and payment
  price            Server-side price calculator
  stack-detect     Detect language/framework for a (repo, ref)
  github           GitHub App integration (OAuth, repos, branches)
  orgs             Manage organizations, members, invitations
  admin            Admin-only user management

All commands support --json for machine-readable output, making the CLI
suitable for scripting and AI agent automation.

Aliases (all equivalent):
  machines = machine = m = projects = project = p

Quick Start:
  1. usectl login                                     # Authenticate
  2. usectl github login                              # Connect GitHub
  3. usectl machines create my-app --vcpu 2 --ram 4 --storage 10
  4. usectl machines pods create my-app \
       --repo https://github.com/user/repo --port 3000
  5. usectl machines pods my-app                      # Config + running pods
  6. usectl machines logs my-app -f                   # Tail logs

──────────────────────────────────────────────────────────────────────────
AI AGENT GUIDE
──────────────────────────────────────────────────────────────────────────

Mental model — a MACHINE and a POD are different things:

  A MACHINE is a Kubernetes namespace (kdeploy-<name>) plus a resource
  wallet: vCPU, RAM, storage, and one billing subscription. It holds no
  code. Its name must be unique platform-wide, because the namespace is
  derived from it.

  A POD is one workload inside a machine. Everything about *running code*
  belongs here: git repo and branch (or a prebuilt image), domain, primary
  and extra ports, public/internal visibility, CPU/memory/storage limits,
  rollout strategy, replicas.

  So repo, branch, domain and port are POD flags, never machine flags:
    usectl machines create api --vcpu 2 --ram 4 --storage 10
    usectl machines pods create api --repo <url> --branch main --port 8080

  ADDONS (postgres, redis, nats, mongodb, s3, ...) are provisioned per
  machine; their credentials are auto-injected into every pod as env vars
  (DATABASE_URL, REDIS_URL, ...). Never set those by hand.

  GROUPS partition a machine's pods and addons into sibling namespaces with
  NetworkPolicy isolation between them. Group names are unique per machine.

Non-interactive use:
  Commands that would otherwise prompt (machines create, pods create) never
  prompt when --json or --yes is set, or when stdin is not a terminal.
  They exit non-zero listing exactly which flags were missing, so an agent
  is never blocked on an invisible prompt.

Do I need to commit a Dockerfile?
  Run 'usectl stack-detect --repo <url> --ref <branch>' first. Auto-build
  rules (no Dockerfile required, port is forced to 80):
    next.config.*    → Next.js (npm run build → next start -p 80)
    vite.config.*    → Vite/React/Vue (npm run build → nginx serves /dist)
    package.json     → Generic Node.js (npm start, port 80)
    only HTML/CSS/JS → Static site served by nginx
  Everything else (Go, Python, Rust, Java, custom Node, ...) MUST ship a
  Dockerfile in the repo root that EXPOSEs its port, and the pod must pass
  --port matching that EXPOSE.

Common recipes:
  # Machine sized for a small API, with Postgres:
  usectl machines create api --vcpu 2 --ram 4 --storage 20 --db

  # Its web pod (Dockerfile in repo, listens on 8080):
  usectl machines pods create api --name web \
    --repo https://github.com/me/api --port 8080

  # A worker pod in the same machine, sharing its addons:
  usectl machines pods create api --name worker \
    --repo https://github.com/me/api --kind worker \
    --command "go run ./worker"`,
}

func Execute() {
	// Runs after every package init(), so the machine-scoped grouping is
	// independent of the filename ordering init() would otherwise impose.
	attachMachineScopedCommands()

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	// So the API can attribute variable changes to the CLI rather than the
	// dashboard (USCT-192). Set here rather than in api/ to avoid an import
	// cycle — cmd already imports api.
	api.ClientVersion = Version

	rootCmd.PersistentFlags().StringVar(&apiURL, "api-url", "", "API base URL (default: from config or https://manager.usectl.com)")
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	rootCmd.PersistentFlags().StringVar(&colorMode, "color", "auto", "Colourise output: auto, always, never")

	// Applied before any command runs so every renderer sees the final
	// setting. "auto" leaves the terminal/NO_COLOR detection in place.
	cobra.OnInitialize(func() {
		switch colorMode {
		case "always":
			output.SetColor(true)
		case "never":
			output.SetColor(false)
		}
		// --json output must never carry escape sequences: it is parsed, not read.
		if jsonOutput {
			output.SetColor(false)
		}
	})
}
