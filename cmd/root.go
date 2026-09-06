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
var Version = "v2.0.1"

var rootCmd = &cobra.Command{
	Use:     "usectl",
	Short:   "usectl — CLI for the usectl.com deployment platform",
	Version: Version,
	Long: `usectl is the CLI for the usectl.com platform. It provides full lifecycle
management for your applications on Kubernetes.

Command Groups:
  login / logout   Authenticate (browser by default; --password for CI)
  register         Create an account
  profile          View and update your user profile
  use              Remember a default machine (and pod) for later commands
  schema           Print the whole command tree as JSON, for scripts and agents

  machines             Create, resize, delete and monitor machines
  machines pods        Every pod: config, limits, node, addons, ports, env
  machines addons      Addons (database, redis, nats, mongodb, s3, ...)
  machines envs        Environment variables, machine-wide or per pod
  machines groups      Partition pods/addons into isolated group namespaces
  machines members     Members, roles, invitations, per-developer scope
  machines domains     Custom domains, including per-pod pinning
  machines cron        Scheduled cron jobs and run history
  machines quota       Quota, recommendations, resize previews
  machines usage       Allocation against the plan's limits
  machines settings    Machine-level configuration
  machines vars        Build-time vs runtime variable exposure
  machines s3          Object storage for a machine
  machines enter       Interactive sub-shell scoped to one machine

  deployments      Deployment history; roll back to a previous build
  domains          Register, attach and verify domains you own
  project-domains  List the domains attached to one machine
  registry         Container-image allowance and what is consuming it
  images           Manage image-sourced pods' references
  notifications    View and acknowledge in-app notifications
  billing          Manage your subscription and payment
  project-billing  Per-machine billing status and portal
  price            Server-side price calculator
  storage          Storage usage for a machine
  stack-detect     Detect language/framework for a (repo, ref)
  trial-status     Show your current trial countdown
  github / google  OAuth integrations
  orgs             Manage organizations, members, invitations
  admin            Admin-only user management
  mcp              Model Context Protocol config for AI assistants

Every machine-scoped group is reachable two ways: 'usectl machines addons ...'
and 'usectl addons ...' run the same command.

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

Start here: 'usectl schema --json' prints the entire command tree — every
command, alias, argument shape and flag — in one call. It is generated from
the live tree, so it always matches this binary, and it carries notes on
target resolution, reachability, and which commands are destructive. Read it
instead of scraping --help recursively.

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
  machine, but a pod only receives their credentials once the addon is
  ATTACHED to it. A pod with NO attachments inherits every addon in the
  machine; a pod with some attachments receives only those. Never set
  DATABASE_URL, REDIS_URL and the like by hand — they are injected.

REACHABILITY — a pod is not reachable until it has a domain:

  Creating a pod does NOT publish it. Without a domain the pod gets a
  ClusterIP Service and no IngressRoute, so it is reachable only from other
  pods in the same machine, by its name. Nothing outside the cluster — a
  browser, a webhook, another machine, a frontend calling an API — can
  reach it.

  So whenever the user wants something reachable, attach a domain:

    usectl machines pods create api web --repo <url> --port 8080 --domain api
    usectl machines pods set api web domain=api.example.com

  A value with NO dot becomes a platform subdomain (api -> api.usectl.com).
  A value WITH a dot is used as the user's own domain, and needs its DNS
  pointed at the platform.

  Leave the domain off only when the pod is genuinely internal — a worker,
  a queue consumer, or a backend that is called by a sibling pod rather
  than from outside. Pair that with --private, which states the intent.

  'usectl machines pods <machine>' reports this per pod on the 'visibility'
  line: "public -> host" when reachable, "public (no domain attached)" when
  it is not, which is a pod nothing can call.

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
