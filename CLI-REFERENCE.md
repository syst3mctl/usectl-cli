# usectl CLI — Commands & Flags Reference

Complete syntax reference for the `usectl` command-line client: every command,
its real arguments, its flags, and how it behaves when things go wrong.

Generated from the compiled binary's own help output, so the syntax below is
what the CLI actually accepts — not a hand-maintained list that has drifted
from the code.

- **Binary:** `usectl`
- **Module:** `github.com/giorgi/usectl`
- **Default API:** `https://manager.usectl.com`
- **Config file:** `~/.usectl/config.json`
- **Generated:** 2026-09-01

---

## Terminology

A **machine** is one Kubernetes namespace (`kdeploy-<name>`) plus one domain.
The dashboard renamed "project" to "machine"; the CLI follows, and
`usectl projects …` / `usectl p …` remain working aliases so existing scripts
keep running.

An **app** is a pod inside a machine, built from its own repo and branch. A
machine can hold several. Apps come in three kinds — `web` (accepts traffic),
`worker` (no inbound port, needs a command), and `release` (runs once per
deploy, e.g. migrations).

An **addon** is managed infrastructure attached to a machine: Postgres, Redis,
NATS, MongoDB, S3/MinIO, Meilisearch, MSSQL, OAuth2 Proxy, Grafana, backup,
dbui, cron.

A **group** partitions a machine's apps and addons into a sibling namespace
(`kdeploy-<machine>-<group>`) isolated by NetworkPolicy.

---

## Global flags

Available on every command:

```
--api-url string   API base URL (default: from config, else https://manager.usectl.com)
--json             Machine-readable JSON output
-h, --help         Help for any command
-v, --version      Print the CLI version
```

`--json` is the flag that matters for scripting and agents: it switches every
command from human tables to raw JSON, including error payloads.

---

## Authentication

```bash
usectl login                  # interactive email + password
usectl register               # create an account (if native registration is enabled)
usectl github login           # connect GitHub via the GitHub App
usectl logout                 # clear the local session
```

Login stores an access token **and** an opaque refresh token in
`~/.usectl/config.json`. The client refreshes transparently on a `401` and
re-saves the rotated refresh token.

Refresh tokens rotate on every use. Presenting an already-used token is treated
as a leaked credential and revokes the whole token family, so **if the CLI
cannot write its config file, the next refresh looks like a replay and the
session is killed.** A read-only `$HOME` is the usual cause.

---

## Exit codes

The CLI uses two exit codes:

| Code | Meaning |
|------|---------|
| `0`  | Command succeeded |
| `1`  | Any failure — bad arguments, auth failure, API error, network error |

There is deliberately no richer scheme today: a failed API call, an invalid
flag, and an expired session all exit `1`. Scripts that need to distinguish
them should use `--json` and inspect the payload rather than branching on the
exit status.

Error text goes to **stderr**; command output goes to **stdout**, so
`usectl ... --json > out.json` captures only the payload.

---

## Common workflows

**Deploy from a Git repo**

```bash
usectl login
usectl github login
usectl machines create --name my-app \
  --repo https://github.com/me/repo \
  --domain my-app --port 3000
usectl machines deploy <machine-id>
usectl machines logs <machine-id> -f
```

**Deploy a prebuilt image (no build step)**

```bash
usectl apps create <machine-id> --name api --source image \
  --image registry.example.com/team/api:v1 --port 8080
usectl images push <machine-id> <app-id> myapp:v1   # or upload a local image
usectl deployments list <machine-id>
```

The image must listen on the port configured for the app. The platform does
**not** inject `PORT`, and a distroless `nonroot` image cannot bind a
privileged port, so `80` will not work for a typical uploaded image.

**Inspect and recover**

```bash
usectl machines pods <machine-id>              # app-level view
usectl kpods list <machine-id>                 # raw Kubernetes pods
usectl kpods delete <machine-id> <pod-name>    # restart one wedged replica
usectl deployments list <machine-id>
usectl deployments rollback <machine-id> <app-id> <deployment-id>
```

---

## Notes that affect real usage

**Variables.** A variable can be marked *protected*, after which its value is
never returned by any API — not to the dashboard, not to `--json`, not to the
reveal endpoint. Protected entries serialise as `"value": null`, deliberately
distinct from `""`, so a script piping `--json` into a `.env` file can tell
"not allowed to read this" from "set to empty" and fail loudly rather than
writing a blank credential.

**Rollback.** Rollback targets an app, not a machine — each app has its own
image, so there is no single image a machine-wide rollback could mean. The
image is read from the stored deployment record rather than recomputed, so
what redeploys is the build that actually ran. If retention has reclaimed that
image the API refuses with `410`; `deployments list` marks those rows
`image reclaimed` so it is visible beforehand.

**Cancellation.** `cancelled` is terminal. Cancelling during the deploy phase
does not merely abort — the rollout already belongs to Kubernetes, so each
affected app is reverted to its previous live image, or its Deployment is
removed when there is no previous revision.

**Groups.** Groups cannot be renamed: a group is a Kubernetes namespace, and
namespaces cannot be renamed. Delete, recreate, reassign. Cross-group traffic
is dropped by NetworkPolicy, including between a group and the ungrouped
namespace — which requires a NetworkPolicy-enforcing CNI (Calico, Cilium,
Canal). Plain Flannel accepts the resource but does not enforce it.

---

# Command reference

Every command below is reproduced from the binary's own `--help`.

## `usectl addons`

An addon is a packaged service the platform provisions and wires into your


### `usectl addons`

An addon is a packaged service the platform provisions and wires into your

```
usectl addons [command]
```

Aliases: `addons, addon`


Flags:

```
  -h, --help   help for addons
```

### `usectl addons add`

Provision an addon for a machine. Managed mode wires the machine to a

```
usectl addons add <project-id> [flags]
```

Flags:

```
      --config string        Raw JSON merged into dedicated_config
  -h, --help                 help for add
      --mode string          Mode: managed | dedicated (default "managed")
      --name string          Instance name (e.g. 'analytics' for a 2nd database). Empty = primary.
      --replicas int         Replica count (dedicated mode) (default 1)
      --shared-from string   Source addon UUID to share from (managed mode only)
      --size string          Size preset (dedicated mode): small | medium | large | ...
      --type string          Addon type: database, redis, nats, mongodb, s3, ... (required)
      --version string       Addon version override (dedicated mode)
```

### `usectl addons catalog`

List all addons available in the catalog

```
usectl addons catalog [flags]
```

Flags:

```
  -h, --help   help for catalog
```

### `usectl addons config`

Sends a JSON object that is merged into the addon row's config column. Used

```
usectl addons config <project-id> <addon-id> <json> [flags]
```

Flags:

```
  -h, --help   help for config
```

### `usectl addons list`

List addons enabled on a project

```
usectl addons list <project-id> [flags]
```

Flags:

```
  -h, --help   help for list
```

### `usectl addons remove`

Remove an addon (by type for the primary instance, or by UUID for any instance)

```
usectl addons remove <project-id> <type-or-id> [flags]
```

Aliases: `remove, rm`


Flags:

```
  -h, --help   help for remove
```

### `usectl addons shareable`

List addons from your other projects you can share-link to this one

```
usectl addons shareable <project-id> [flags]
```

Flags:

```
  -h, --help   help for shareable
```

### `usectl addons start`

Restore a dedicated addon's replicas (no-op for managed)

```
usectl addons start <project-id> <addon-id> [flags]
```

Flags:

```
  -h, --help   help for start
```

### `usectl addons stop`

Scale a dedicated addon to 0 (no-op for managed)

```
usectl addons stop <project-id> <addon-id> [flags]
```

Flags:

```
  -h, --help   help for stop
```

### `usectl addons ui`

Enable or disable the addon's admin UI (pgAdmin, Redis Commander, ...)

```
usectl addons ui <project-id> <type-or-id> [flags]
```

Flags:

```
      --disable   Disable the addon's admin UI
      --enable    Enable the addon's admin UI
  -h, --help      help for ui
```

## `usectl admin`

Admin commands (user management)


### `usectl admin`

Admin commands (user management)

```
usectl admin [command]
```

Flags:

```
  -h, --help   help for admin
```

### `usectl admin users`

Manage users

```
usectl admin users [command]
```

Flags:

```
  -h, --help   help for users
```

### `usectl admin users delete`

Delete a user

```
usectl admin users delete <id> [flags]
```

Flags:

```
  -h, --help   help for delete
```

### `usectl admin users disable`

Disable a user

```
usectl admin users disable <id> [flags]
```

Flags:

```
  -h, --help   help for disable
```

### `usectl admin users enable`

Enable a user

```
usectl admin users enable <id> [flags]
```

Flags:

```
  -h, --help   help for enable
```

### `usectl admin users list`

List all users

```
usectl admin users list [flags]
```

Aliases: `list, ls`


Flags:

```
  -h, --help   help for list
```

### `usectl admin users set-role`

Set user role (user or admin)

```
usectl admin users set-role <id> <role> [flags]
```

Flags:

```
  -h, --help   help for set-role
```

## `usectl apps`

Each project can host multiple "apps" — independent pods built from their


### `usectl apps`

Each project can host multiple "apps" — independent pods built from their

```
usectl apps [command]
```

Aliases: `apps, app`


Flags:

```
  -h, --help   help for apps
```

### `usectl apps addons`

Manage which addons inject env vars into a single app

```
usectl apps addons [command]
```

Flags:

```
  -h, --help   help for addons
```

### `usectl apps addons attach`

Attach an addon to an app (inject its env vars)

```
usectl apps addons attach <project-id> <app-id> <addon-id> [flags]
```

Flags:

```
  -h, --help   help for attach
```

### `usectl apps addons detach`

Detach an addon from an app (stop injecting its env vars)

```
usectl apps addons detach <project-id> <app-id> <addon-id> [flags]
```

Flags:

```
  -h, --help   help for detach
```

### `usectl apps addons list`

List addons attached to a single app

```
usectl apps addons list <project-id> <app-id> [flags]
```

Flags:

```
  -h, --help   help for list
```

### `usectl apps create`

Create a web/worker/release pod inside an existing project. Each app

```
usectl apps create <project-id> [flags]
```

Flags:

```
      --arg strings                Container arg (repeatable)
      --auto-deploy                Auto-deploy on push
      --branch string              Git branch (default "main")
      --command string             Override container command (required for worker/release)
      --domain string              Subdomain
      --extra-port stringArray     Extra cluster-internal-only port [name:]port[/proto], e.g. grpc:9094/tcp (repeatable, web pods only)
  -h, --help                       help for create
      --image string               Deploy a prebuilt image instead of building from a repo, e.g. ghcr.io/acme/api:v1.2.3
      --installation-id int        GitHub App installation ID
      --kind string                Pod kind: web, worker, release (default "web")
      --metrics                    Scrape this app's /metrics endpoint into the platform metrics store (web pods only)
      --metrics-path string        Path serving Prometheus text format (default /metrics)
      --metrics-port int           Port serving metrics (default: the app's primary port; must be the primary port or an --extra-port)
      --name string                App name (required)
      --port int                   Container port (default 3000)
      --preview-envs               Enable PR preview envs for this app
      --private                    Private app (cluster-internal only, no IngressRoute)
      --registry-password string   Private registry password or token (with --image)
      --registry-user string       Private registry username (with --image)
      --replicas int               Replica count (default 1)
      --repo string                GitHub repo URL (required unless --image)
```

### `usectl apps delete`

Delete an app and tear down its K8s resources

```
usectl apps delete <project-id> <app-id> [flags]
```

Flags:

```
  -h, --help   help for delete
```

### `usectl apps envs`

Manage per-app environment variables (override project envs)

```
usectl apps envs [command]
```

Flags:

```
  -h, --help   help for envs
```

### `usectl apps envs delete`

Delete one or more per-app env vars

```
usectl apps envs delete <project-id> <app-id> KEY [KEY ...] [flags]
```

Flags:

```
  -h, --help   help for delete
```

### `usectl apps envs list`

List per-app + inherited project env vars

```
usectl apps envs list <project-id> <app-id> [flags]
```

Flags:

```
  -h, --help   help for list
```

### `usectl apps envs protect`

Control whether a per-app variable's value can be read back.

```
usectl apps envs protect <project-id> <app-id> <protect|open> KEY [KEY ...] [flags]
```

Flags:

```
  -h, --help   help for protect
```

### `usectl apps envs set`

Set one or more per-app env vars

```
usectl apps envs set <project-id> <app-id> KEY=VALUE [KEY=VALUE ...] [flags]
```

Flags:

```
  -h, --help   help for set
```

### `usectl apps insights`

Show per-pod CPU/memory history + recent error logs

```
usectl apps insights <project-id> <app-id> [flags]
```

Flags:

```
  -h, --help   help for insights
```

### `usectl apps internal`

Show the cluster-internal address for app-to-app calls

```
usectl apps internal <project-id> <app-id> [flags]
```

Flags:

```
  -h, --help   help for internal
```

### `usectl apps list`

List all apps in a project

```
usectl apps list <project-id> [flags]
```

Flags:

```
  -h, --help   help for list
```

### `usectl apps resize`

Change how much memory and/or CPU each replica of this app gets.

```
usectl apps resize <project-id> <app-id> [flags]
```

Flags:

```
      --cpu string        Per-pod CPU (e.g. 1, 0.5, 500m)
  -h, --help              help for resize
      --memory string     Per-pod memory (e.g. 1Gi, 512Mi, 768)
      --storage string    Per-pod ephemeral storage (e.g. 4Gi, 2Gi). Default 2 GiB.
      --strategy string   Rollout strategy: 'rolling' (zero-downtime) or 'recreate' (no surge, brief downtime)
```

### `usectl apps restart`

Rolling restart (no rebuild) — picks up new env vars

```
usectl apps restart <project-id> <app-id> [flags]
```

Flags:

```
  -h, --help   help for restart
```

### `usectl apps reveal`

Reveal the unmasked value of a single variable (audited)

```
usectl apps reveal <project-id> <app-id> <key> [flags]
```

Flags:

```
  -h, --help   help for reveal
```

### `usectl apps start`

Start an app (restore replicas from 0)

```
usectl apps start <project-id> <app-id> [flags]
```

Flags:

```
  -h, --help   help for start
```

### `usectl apps stop`

Stop an app (scale Deployment to 0)

```
usectl apps stop <project-id> <app-id> [flags]
```

Flags:

```
  -h, --help   help for stop
```

### `usectl apps traffic`

Show Traefik request metrics (rate, p50/p95/p99, bytes, codes)

```
usectl apps traffic <project-id> <app-id> [flags]
```

Flags:

```
  -h, --help   help for traffic
```

### `usectl apps update`

Update settings on an existing app

```
usectl apps update <project-id> <app-id> [flags]
```

Flags:

```
      --args strings               Container args
      --auto-deploy                Auto-deploy on push
      --branch string              New branch
      --command string             Override container command
      --domain string              New subdomain
      --dotenv-auto                Auto-write .env into container working directory
      --dotenv-path string         Path to write .env file inside container (e.g. "/var/www/html/.env")
      --extra-port stringArray     Replace extra cluster-internal-only ports; [name:]port[/proto] (repeatable, web pods only)
  -h, --help                       help for update
      --image string               Switch to a prebuilt image, e.g. ghcr.io/acme/api:v1.2.3 (stops GitHub deploys)
      --kind string                Pod kind: web, worker, release
      --metrics                    Enable/disable scraping this app's /metrics endpoint
      --metrics-path string        Path serving Prometheus text format (empty resets to /metrics)
      --metrics-port int           Port serving metrics (0 resets to the app's primary port)
      --port int                   New port
      --preview-envs               Enable PR preview envs
      --private                    Make app private (no IngressRoute)
      --registry-password string   Private registry password or token (with --image)
      --registry-user string       Private registry username (with --image)
      --replicas int               New replica count
      --repo string                Switch back to building from this GitHub repo URL
```

### `usectl apps variables`

Show resolved env vars (user + addon-injected, masked by default)

```
usectl apps variables <project-id> <app-id> [flags]
```

Aliases: `variables, vars`


Flags:

```
  -h, --help   help for variables
```

## `usectl billing`

View your current plan, subscription status, and manage billing.


### `usectl billing`

View your current plan, subscription status, and manage billing.

```
usectl billing [command]
```

Aliases: `billing, bill, b`


Flags:

```
  -h, --help   help for billing
```

### `usectl billing portal`

Open Stripe billing portal to manage payment methods (opens browser)

```
usectl billing portal [flags]
```

Flags:

```
  -h, --help   help for portal
```

### `usectl billing status`

View your current billing status and plan

```
usectl billing status [flags]
```

Flags:

```
  -h, --help   help for status
```

### `usectl billing subscribe`

Opens the Stripe checkout page in your browser to subscribe.

```
usectl billing subscribe [plan] [flags]
```

Flags:

```
  -h, --help   help for subscribe
```

## `usectl cron`

Manage Kubernetes CronJobs for a machine. Cron jobs run on a schedule


### `usectl cron`

Manage Kubernetes CronJobs for a machine. Cron jobs run on a schedule

```
usectl machines cron [command]
```

Aliases: `cron, crons, cronjob`


Flags:

```
  -h, --help   help for cron
```

### `usectl cron add`

Create a scheduled cron job. The job runs in the same container image

```
usectl machines cron add <project-id> [flags]
```

Flags:

```
      --command string    Command to run (required)
  -h, --help              help for add
      --name string       Cron job name (required)
      --schedule string   Cron schedule expression (required)
```

### `usectl cron delete`

Delete a cron job

```
usectl machines cron delete <project-id> <cron-id> [flags]
```

Aliases: `delete, rm, remove`


Flags:

```
  -h, --help   help for delete
```

### `usectl cron history`

View past cron job runs (status, duration, logs)

```
usectl machines cron history <project-id> [flags]
```

Flags:

```
      --cron string     Filter by cron name
      --from string     RFC3339 start time
  -h, --help            help for history
      --limit int       Items per page (max 100) (default 10)
      --page int        Page number (default 1)
      --status string   Filter: succeeded | failed | running
      --to string       RFC3339 end time
```

### `usectl cron list`

List all cron jobs for a machine

```
usectl machines cron list <project-id> [flags]
```

Aliases: `list, ls`


Flags:

```
  -h, --help   help for list
```

### `usectl cron update`

Update a cron job's schedule, command, or enabled state

```
usectl machines cron update <project-id> <cron-id> [flags]
```

Flags:

```
      --command string    New command
      --enabled string    Enable or disable (true/false)
  -h, --help              help for update
      --schedule string   New schedule
```

## `usectl deployments`

Deployment history for a machine, newest first.


### `usectl deployments`

Deployment history for a machine, newest first.

```
usectl deployments [command]
```

Aliases: `deployments, deploys, deployment`


Flags:

```
  -h, --help   help for deployments
```

### `usectl deployments list`

List a machine's deployments, newest first

```
usectl deployments list <machine-id> [flags]
```

Flags:

```
      --app string      Filter to one app id
  -h, --help            help for list
      --page int        Page number (default 1)
      --per-page int    Results per page (max 100)
      --status string   Filter by status (live, building, failed, cancelled)
```

### `usectl deployments rollback`

Rolls one app back to the image a previous deployment ran.

```
usectl deployments rollback <machine-id> <app-id> <deployment-id> [flags]
```

Flags:

```
  -h, --help            help for rollback
      --reason string   Reason recorded on the rollback
  -y, --yes             Skip the confirmation prompt
```

## `usectl domains`

Manage custom domains


### `usectl domains`

Manage custom domains

```
usectl machines domains [command]
```

Aliases: `domains, domain, d`


Flags:

```
  -h, --help   help for domains
```

### `usectl domains attach`

Attach one or more free (unattached) domains to a machine.

```
usectl machines domains attach <domain-id> [domain-id...] [flags]
```

Flags:

```
  -h, --help             help for attach
      --project string   Project ID to attach (required)
```

### `usectl domains attach-app`

Project-level domains route to whichever single-app project is the default.

```
usectl machines domains attach-app <domain-id> [flags]
```

Flags:

```
      --app string   App UUID to pin the domain to
      --detach       Detach the domain from any app (project-level routing)
  -h, --help         help for attach-app
```

### `usectl domains create`

Register a custom domain

```
usectl machines domains create <domain> [flags]
```

Flags:

```
  -h, --help   help for create
```

### `usectl domains delete`

Delete a domain

```
usectl machines domains delete <id> [flags]
```

Flags:

```
  -h, --help   help for delete
```

### `usectl domains get`

Get domain details

```
usectl machines domains get <id> [flags]
```

Flags:

```
  -h, --help   help for get
```

### `usectl domains list`

List all domains

```
usectl machines domains list [flags]
```

Aliases: `list, ls`


Flags:

```
  -h, --help   help for list
```

### `usectl domains verify`

Re-check a domain's CNAME / SSL status (Cloudflare for SaaS)

```
usectl machines domains verify <domain-id> [flags]
```

Flags:

```
  -h, --help   help for verify
```

## `usectl envs`

Manage custom environment variables that are securely stored in an encrypted


### `usectl envs`

Manage custom environment variables that are securely stored in an encrypted

```
usectl machines envs [command]
```

Aliases: `envs, env`


Flags:

```
  -h, --help   help for envs
```

### `usectl envs delete`

Delete specific environment variables

```
usectl machines envs delete <project-id> KEY [KEY ...] [flags]
```

Aliases: `delete, rm, remove, unset`


Flags:

```
  -h, --help   help for delete
```

### `usectl envs list`

List all custom environment variables for a machine

```
usectl machines envs list <project-id> [flags]
```

Flags:

```
  -h, --help   help for list
```

### `usectl envs protect`

Control whether a variable's value can be read back.

```
usectl machines envs protect <project-id> <protect|open> KEY [KEY ...] [flags]
```

Flags:

```
  -h, --help   help for protect
```

### `usectl envs set`

Set one or more environment variables for a machine. Uses merge behavior —

```
usectl machines envs set <project-id> KEY=value [KEY=value ...] [flags]
```

Flags:

```
  -h, --help   help for set
```

## `usectl github`

Manage GitHub App integration for private repository access.


### `usectl github`

Manage GitHub App integration for private repository access.

```
usectl github [command]
```

Aliases: `github, gh`


Flags:

```
      --github-token string   GitHub user token (default: from config)
  -h, --help                  help for github
```

### `usectl github app-info`

Display the GitHub App client ID used for OAuth

```
usectl github app-info [flags]
```

Flags:

```
  -h, --help   help for app-info
```

### `usectl github branches`

Returns all branches for a repository, including whether each is protected.

```
usectl github branches [flags]
```

Flags:

```
  -h, --help               help for branches
      --installation int   Installation ID (required)
      --repo string        Repository in owner/name format (required)
```

### `usectl github installations`

List all GitHub App installations for your authenticated GitHub account.

```
usectl github installations [flags]
```

Aliases: `installations, installs`


Flags:

```
  -h, --help   help for installations
```

### `usectl github login`

Opens your browser to authorize with GitHub, then automatically saves the token locally.

```
usectl github login [flags]
```

Flags:

```
  -h, --help   help for login
```

### `usectl github repos`

Returns all repositories that the GitHub App installation has access to.

```
usectl github repos [flags]
```

Flags:

```
  -h, --help               help for repos
      --installation int   Installation ID (required)
```

## `usectl google`

Google Sign In is an alternative to email + password authentication.


### `usectl google`

Google Sign In is an alternative to email + password authentication.

```
usectl google [command]
```

Flags:

```
  -h, --help   help for google
```

### `usectl google login`

Sign into usectl with Google (browser OAuth)

```
usectl google login [flags]
```

Flags:

```
  -h, --help   help for login
```

## `usectl groups`

A group is a sibling namespace, kdeploy-<machine>-<group>, with a NetworkPolicy


### `usectl groups`

A group is a sibling namespace, kdeploy-<machine>-<group>, with a NetworkPolicy

```
usectl groups [command]
```

Aliases: `groups, group`


Flags:

```
  -h, --help   help for groups
```

### `usectl groups create`

Creates a group and its namespace.

```
usectl groups create <machine-id> <name> [flags]
```

Flags:

```
      --color string   Display colour (hex, e.g. #4f46e5)
  -h, --help           help for create
      --sort int       Sort order in the dashboard
```

### `usectl groups delete`

Delete a group

```
usectl groups delete <machine-id> <group-id> [flags]
```

Flags:

```
  -h, --help   help for delete
  -y, --yes    Skip the confirmation prompt
```

### `usectl groups list`

List a machine's groups

```
usectl groups list <machine-id> [flags]
```

Flags:

```
  -h, --help   help for list
```

## `usectl images`

Ship prebuilt container images to your machines


### `usectl images`

Ship prebuilt container images to your machines

```
usectl images [command]
```

Flags:

```
  -h, --help   help for images
```

### `usectl images push`

Push an image you built locally into the platform registry.

```
usectl images push <project-id> <app-id> <local-image> [flags]
```

Flags:

```
      --file string   Upload an existing tar instead of running docker save
  -h, --help          help for push
      --keep-tar      Keep the temporary tar produced by docker save
      --tag string    Tag to use in the platform registry (default: the local tag, else a timestamp)
```

## `usectl kpods`

The Kubernetes view of a machine, including addon pods, group namespaces,


### `usectl kpods`

The Kubernetes view of a machine, including addon pods, group namespaces,

```
usectl kpods [command]
```

Aliases: `kpods, k8s-pods`


Flags:

```
  -h, --help   help for kpods
```

### `usectl kpods delete`

Deletes a single pod. The Deployment or StatefulSet that owns it creates a

```
usectl kpods delete <machine-id> <pod-name> [flags]
```

Aliases: `delete, restart`


Flags:

```
  -h, --help   help for delete
  -y, --yes    Skip the confirmation prompt
```

### `usectl kpods list`

List every pod in the machine's namespaces

```
usectl kpods list <machine-id> [flags]
```

Flags:

```
  -h, --help   help for list
```

## `usectl login`

Interactively log in with your email and password. The JWT token is saved


### `usectl login`

Interactively log in with your email and password. The JWT token is saved

```
usectl login [flags]
```

Flags:

```
  -h, --help   help for login
```

## `usectl logout`

Log out of usectl.


### `usectl logout`

Log out of usectl.

```
usectl logout [flags]
```

Flags:

```
      --all    revoke every session on the account, not just this device
  -h, --help   help for logout
```

## `usectl machines`

Manage the full lifecycle of applications on the usectl platform.


### `usectl machines`

Manage the full lifecycle of applications on the usectl platform.

```
usectl machines [command]
```

Aliases: `machines, machine, m, projects, project, p`


Flags:

```
  -h, --help   help for machines
```

### `usectl machines build-logs`

Retrieve the full build log (clone + Kaniko build) and deploy log for a

```
usectl machines build-logs <project-id> <deployment-id> [flags]
```

Flags:

```
  -h, --help   help for build-logs
```

### `usectl machines create`

Create a new project linked to a GitHub repository. The project will be

```
usectl machines create [flags]
```

Flags:

```
      --addon strings         Add addon by type (database, s3, redis, nats). Can be repeated
      --branch string         Git branch (default "main")
      --db                    Provision a PostgreSQL database
      --domain string         Subdomain (required)
      --env strings           Set environment variable (KEY=value). Can be repeated
      --github-token string   GitHub token for private repos
  -h, --help                  help for create
      --installation-id int   GitHub App installation ID (from 'usectl github installations')
      --name string           Project name (required)
      --port int              Container port (default 80)
      --repo string           GitHub repository URL (required)
      --s3                    Provision S3 object storage
      --type string           Project type: static or service (default "service")
```

### `usectl machines cron`

Manage Kubernetes CronJobs for a machine. Cron jobs run on a schedule

```
usectl machines cron [command]
```

Aliases: `cron, crons, cronjob`


Flags:

```
  -h, --help   help for cron
```

### `usectl machines cron add`

Create a scheduled cron job. The job runs in the same container image

```
usectl machines cron add <project-id> [flags]
```

Flags:

```
      --command string    Command to run (required)
  -h, --help              help for add
      --name string       Cron job name (required)
      --schedule string   Cron schedule expression (required)
```

### `usectl machines cron delete`

Delete a cron job

```
usectl machines cron delete <project-id> <cron-id> [flags]
```

Aliases: `delete, rm, remove`


Flags:

```
  -h, --help   help for delete
```

### `usectl machines cron history`

View past cron job runs (status, duration, logs)

```
usectl machines cron history <project-id> [flags]
```

Flags:

```
      --cron string     Filter by cron name
      --from string     RFC3339 start time
  -h, --help            help for history
      --limit int       Items per page (max 100) (default 10)
      --page int        Page number (default 1)
      --status string   Filter: succeeded | failed | running
      --to string       RFC3339 end time
```

### `usectl machines cron list`

List all cron jobs for a machine

```
usectl machines cron list <project-id> [flags]
```

Aliases: `list, ls`


Flags:

```
  -h, --help   help for list
```

### `usectl machines cron update`

Update a cron job's schedule, command, or enabled state

```
usectl machines cron update <project-id> <cron-id> [flags]
```

Flags:

```
      --command string    New command
      --enabled string    Enable or disable (true/false)
  -h, --help              help for update
      --schedule string   New schedule
```

### `usectl machines delete`

Permanently delete a project and clean up all associated resources:

```
usectl machines delete <id> [flags]
```

Flags:

```
  -h, --help   help for delete
```

### `usectl machines deploy`

Trigger a build pipeline for the project. The backend auto-resolves the

```
usectl machines deploy <id> [flags]
```

Flags:

```
  -h, --help   help for deploy
```

### `usectl machines deployments`

Returns a table of all deployments for the given project, ordered from

```
usectl machines deployments <project-id> [flags]
usectl machines deployments [command]
```

Aliases: `deployments, deps`


Flags:

```
  -h, --help   help for deployments
```

### `usectl machines deployments cancel`

Cancel a running deployment (kills the build job)

```
usectl machines deployments cancel <project-id> <deployment-id> [flags]
```

Flags:

```
  -h, --help   help for cancel
```

### `usectl machines diagnostics`

Returns precise Kubernetes pod lifecycle events to debug crash loops or CreateContainerConfigErrors.

```
usectl machines diagnostics <id> [flags]
```

Flags:

```
  -h, --help   help for diagnostics
```

### `usectl machines domains`

Manage custom domains

```
usectl machines domains [command]
```

Aliases: `domains, domain, d`


Flags:

```
  -h, --help   help for domains
```

### `usectl machines domains attach`

Attach one or more free (unattached) domains to a machine.

```
usectl machines domains attach <domain-id> [domain-id...] [flags]
```

Flags:

```
  -h, --help             help for attach
      --project string   Project ID to attach (required)
```

### `usectl machines domains attach-app`

Project-level domains route to whichever single-app project is the default.

```
usectl machines domains attach-app <domain-id> [flags]
```

Flags:

```
      --app string   App UUID to pin the domain to
      --detach       Detach the domain from any app (project-level routing)
  -h, --help         help for attach-app
```

### `usectl machines domains create`

Register a custom domain

```
usectl machines domains create <domain> [flags]
```

Flags:

```
  -h, --help   help for create
```

### `usectl machines domains delete`

Delete a domain

```
usectl machines domains delete <id> [flags]
```

Flags:

```
  -h, --help   help for delete
```

### `usectl machines domains get`

Get domain details

```
usectl machines domains get <id> [flags]
```

Flags:

```
  -h, --help   help for get
```

### `usectl machines domains list`

List all domains

```
usectl machines domains list [flags]
```

Aliases: `list, ls`


Flags:

```
  -h, --help   help for list
```

### `usectl machines domains verify`

Re-check a domain's CNAME / SSL status (Cloudflare for SaaS)

```
usectl machines domains verify <domain-id> [flags]
```

Flags:

```
  -h, --help   help for verify
```

### `usectl machines envs`

Manage custom environment variables that are securely stored in an encrypted

```
usectl machines envs [command]
```

Aliases: `envs, env`


Flags:

```
  -h, --help   help for envs
```

### `usectl machines envs delete`

Delete specific environment variables

```
usectl machines envs delete <project-id> KEY [KEY ...] [flags]
```

Aliases: `delete, rm, remove, unset`


Flags:

```
  -h, --help   help for delete
```

### `usectl machines envs list`

List all custom environment variables for a machine

```
usectl machines envs list <project-id> [flags]
```

Flags:

```
  -h, --help   help for list
```

### `usectl machines envs protect`

Control whether a variable's value can be read back.

```
usectl machines envs protect <project-id> <protect|open> KEY [KEY ...] [flags]
```

Flags:

```
  -h, --help   help for protect
```

### `usectl machines envs set`

Set one or more environment variables for a machine. Uses merge behavior —

```
usectl machines envs set <project-id> KEY=value [KEY=value ...] [flags]
```

Flags:

```
  -h, --help   help for set
```

### `usectl machines get`

Returns detailed information about a single project, including repo URL,

```
usectl machines get <id> [flags]
```

Flags:

```
  -h, --help   help for get
```

### `usectl machines list`

Returns a table of all projects the authenticated user has access to.

```
usectl machines list [flags]
```

Aliases: `list, ls`


Flags:

```
  -h, --help   help for list
```

### `usectl machines logs`

Fetch the latest log output from the project's running pods.

```
usectl machines logs <id> [flags]
```

Flags:

```
  -f, --follow     Follow log output in real-time
  -h, --help       help for logs
      --tail int   Number of log lines (default 100)
```

### `usectl machines pods`

Pod-level operations for a machine.

```
usectl machines pods <machine-id> [flags]
usectl machines pods [command]
```

Aliases: `pods, pod`


Flags:

```
  -h, --help   help for pods
```

### `usectl machines pods diagnostics`

Show K8s lifecycle events + previous-crash logs for the failing pod

```
usectl machines pods diagnostics <machine-id> [flags]
```

Aliases: `diagnostics, diag`


Flags:

```
  -h, --help   help for diagnostics
```

### `usectl machines pods list`

List pods running inside a machine

```
usectl machines pods list <machine-id> [flags]
```

Aliases: `list, ls`


Flags:

```
  -h, --help   help for list
```

### `usectl machines pods logs`

Same data source as 'machines logs', surfaced under the pods group for

```
usectl machines pods logs <machine-id> [flags]
```

Flags:

```
      --app string   Scope to one app (multi-app machines)
  -f, --follow       Stream logs in real time
  -h, --help         help for logs
      --tail int     Number of lines to fetch (default 100)
```

### `usectl machines pods restart`

Bumps the kubectl.kubernetes.io/restartedAt annotation on each app's

```
usectl machines pods restart <machine-id> [flags]
```

Flags:

```
  -h, --help   help for restart
```

### `usectl machines pods shell`

Connects to a pod via the SPDY-proxy WebSocket tunnel exposed at

```
usectl machines pods shell <machine-id> [flags]
```

Flags:

```
      --app string   Pick the first running pod of this app id
  -h, --help         help for shell
      --pod string   Specific pod name (default: first ready pod)
```

### `usectl machines prs`

Returns a table of active pull request preview environments for the given

```
usectl machines prs <project-id> [flags]
usectl machines prs [command]
```

Aliases: `prs, pr, previews`


Flags:

```
  -h, --help   help for prs
```

### `usectl machines prs delete`

Tear down a PR preview environment

```
usectl machines prs delete <project-id> <pr-number> [flags]
```

Flags:

```
  -h, --help   help for delete
```

### `usectl machines rollback`

Roll back a project to a previously successful deployment by redeploying

```
usectl machines rollback <project-id> <deployment-id> [flags]
```

Flags:

```
  -h, --help   help for rollback
```

### `usectl machines s3`

Manage the S3 bucket provisioned for a machine.

```
usectl machines s3 [command]
```

Flags:

```
  -h, --help   help for s3
```

### `usectl machines s3 cdn`

Enable or disable public CDN access for a machine's S3 bucket.

```
usectl machines s3 cdn <project-id> [flags]
```

Flags:

```
  -h, --help   help for cdn
```

### `usectl machines s3 download`

Download a single object by its key. The file is saved to the current

```
usectl machines s3 download <project-id> [flags]
```

Flags:

```
  -h, --help            help for download
      --key string      Object key to download (required)
      --output string   Output file path (default: filename from key)
```

### `usectl machines s3 list`

Returns all objects (files and directories) in the machine's S3 bucket.

```
usectl machines s3 list <project-id> [flags]
```

Flags:

```
  -h, --help            help for list
      --prefix string   Filter by key prefix (path/)
```

### `usectl machines s3 toggle`

Toggle S3 storage on or off. When enabled, a bucket and dedicated

```
usectl machines s3 toggle <project-id> [flags]
```

Flags:

```
      --enable   Enable S3 (use --enable=false to disable)
  -h, --help     help for toggle
```

### `usectl machines shell`

Upgrades your connection to an interactive SPDY-proxy WebSocket tunnel,

```
usectl machines shell <id> [flags]
```

Flags:

```
  -h, --help   help for shell
```

### `usectl machines start`

Start a stopped project (scale replicas to 1)

```
usectl machines start <id> [flags]
```

Flags:

```
  -h, --help   help for start
```

### `usectl machines stats`

Returns resource usage metrics for the project, including:

```
usectl machines stats <id> [flags]
```

Flags:

```
  -h, --help   help for stats
```

### `usectl machines status`

Returns the running status and current replica count of the project's deployment.

```
usectl machines status <id> [flags]
```

Flags:

```
  -h, --help   help for status
```

### `usectl machines stop`

Stop a running project (scale replicas to 0)

```
usectl machines stop <id> [flags]
```

Flags:

```
  -h, --help   help for stop
```

### `usectl machines update`

Modify one or more settings of an existing project. Only the flags you

```
usectl machines update <id> [flags]
```

Flags:

```
      --branch string         New branch
      --domain string         New subdomain
      --github-token string   New GitHub token
  -h, --help                  help for update
      --installation-id int   GitHub App installation ID
      --name string           New project name
      --port int              New container port
      --preview-envs          Enable or disable PR preview environments
```

## `usectl mcp`

Model Context Protocol (MCP) integrations


### `usectl mcp`

Model Context Protocol (MCP) integrations

```
usectl mcp [command]
```

Flags:

```
  -h, --help   help for mcp
```

### `usectl mcp config`

Generates the JSON block required to connect Claude Desktop to your remote usectl cluster via Server-Sent Events (SSE).

```
usectl mcp config [flags]
```

Flags:

```
  -h, --help   help for config
```

## `usectl members`

Project members are independent of organization roles — a user can be


### `usectl members`

Project members are independent of organization roles — a user can be

```
usectl members [command]
```

Aliases: `members, member, team`


Flags:

```
  -h, --help   help for members
```

### `usectl members invitations`

Manage machine invitations (per-machine, separate from org invites)

```
usectl members invitations [command]
```

Aliases: `invitations, invites`


Flags:

```
  -h, --help   help for invitations
```

### `usectl members invitations accept`

Accept a project invitation token (you must be logged in)

```
usectl members invitations accept <token> [flags]
```

Flags:

```
  -h, --help   help for accept
```

### `usectl members invitations create`

Invite a user to a machine by email. Owner-only.

```
usectl members invitations create <project-id> [flags]
```

Flags:

```
      --email string   Email address (required)
  -h, --help           help for create
      --role string    Role: owner | developer | viewer (default "developer")
```

### `usectl members invitations list`

List pending invitations on a project

```
usectl members invitations list <project-id> [flags]
```

Flags:

```
  -h, --help   help for list
```

### `usectl members invitations revoke`

Revoke a pending invitation. Owner-only.

```
usectl members invitations revoke <project-id> <invitation-id> [flags]
```

Flags:

```
  -h, --help   help for revoke
```

### `usectl members list`

List members of a machine

```
usectl members list <project-id> [flags]
```

Flags:

```
  -h, --help   help for list
```

### `usectl members my-role`

Show my effective role on a project (and resource scope, if any)

```
usectl members my-role <project-id> [flags]
```

Flags:

```
  -h, --help   help for my-role
```

### `usectl members remove`

Remove a member from a machine. Owner-only.

```
usectl members remove <project-id> <user-id> [flags]
```

Aliases: `remove, rm`


Flags:

```
  -h, --help   help for remove
```

### `usectl members role`

Change a member's role (owner|developer|viewer). Owner-only.

```
usectl members role <project-id> <user-id> <role> [flags]
```

Flags:

```
  -h, --help   help for role
```

### `usectl members scope-get`

Show the developer's resource whitelist (apps/addons)

```
usectl members scope-get <project-id> <user-id> [flags]
```

Flags:

```
  -h, --help   help for scope-get
```

### `usectl members scope-set`

Sets the developer's allowed apps/addons. Pass --app/--addon flags

```
usectl members scope-set <project-id> <user-id> [flags]
```

Flags:

```
      --addon strings   Addon UUID (repeatable)
      --app strings     App UUID (repeatable)
      --clear           Clear all restrictions
  -h, --help            help for scope-set
```

## `usectl notifications`

View and manage in-app notifications


### `usectl notifications`

View and manage in-app notifications

```
usectl notifications [command]
```

Aliases: `notifications, notify, notifs`


Flags:

```
  -h, --help   help for notifications
```

### `usectl notifications list`

List recent notifications

```
usectl notifications list [flags]
```

Flags:

```
  -h, --help   help for list
```

### `usectl notifications read`

Mark a notification as read

```
usectl notifications read <id> [flags]
```

Flags:

```
  -h, --help   help for read
```

### `usectl notifications read-all`

Mark all notifications as read

```
usectl notifications read-all [flags]
```

Flags:

```
  -h, --help   help for read-all
```

### `usectl notifications unread`

Show unread notification count

```
usectl notifications unread [flags]
```

Flags:

```
  -h, --help   help for unread
```

## `usectl orgs`

Manage organizations for team-based project collaboration.


### `usectl orgs`

Manage organizations for team-based project collaboration.

```
usectl orgs [command]
```

Aliases: `orgs, org, organizations`


Flags:

```
  -h, --help   help for orgs
```

### `usectl orgs create`

Create a new organization. You will be the owner and can invite others.

```
usectl orgs create [flags]
```

Flags:

```
      --desc string   Organization description
  -h, --help          help for create
      --name string   Organization name (required)
      --slug string   URL-friendly slug (auto-generated from name if omitted)
```

### `usectl orgs delete`

Permanently delete an organization and remove all member associations.

```
usectl orgs delete <org-id> [flags]
```

Flags:

```
  -h, --help   help for delete
```

### `usectl orgs get`

Display detailed information about an organization including name, slug, and creation date.

```
usectl orgs get <org-id> [flags]
```

Flags:

```
  -h, --help   help for get
```

### `usectl orgs invite`

Create, list, and revoke invitations to an organization.

```
usectl orgs invite [command]
```

Aliases: `invite, invitations, inv`


Flags:

```
  -h, --help   help for invite
```

### `usectl orgs invite accept`

Accept a pending invitation using the token. You will be added as a member

```
usectl orgs invite accept <token> [flags]
```

Flags:

```
  -h, --help   help for accept
```

### `usectl orgs invite create`

Send an invitation to a user by email address. The invitation creates a

```
usectl orgs invite create <org-id> [flags]
```

Flags:

```
      --email string   Email address of the user to invite (required)
  -h, --help           help for create
      --role string    Role to assign: admin, member, or viewer (default "member")
```

### `usectl orgs invite info`

Display information about a pending invitation before accepting it.

```
usectl orgs invite info <token> [flags]
```

Flags:

```
  -h, --help   help for info
```

### `usectl orgs invite list`

Show all pending (not yet accepted) invitations for the specified organization.

```
usectl orgs invite list <org-id> [flags]
```

Aliases: `list, ls`


Flags:

```
  -h, --help   help for list
```

### `usectl orgs invite revoke`

Cancel a pending invitation so it can no longer be used to join the organization.

```
usectl orgs invite revoke <org-id> <invitation-id> [flags]
```

Aliases: `revoke, delete, rm`


Flags:

```
  -h, --help   help for revoke
```

### `usectl orgs list`

Show all organizations the current user is a member of, including their role in each.

```
usectl orgs list [flags]
```

Aliases: `list, ls`


Flags:

```
  -h, --help   help for list
```

### `usectl orgs members`

List, update roles, and remove members from an organization.

```
usectl orgs members [command]
```

Aliases: `members, member`


Flags:

```
  -h, --help   help for members
```

### `usectl orgs members list`

Show all members of the specified organization with their roles and join dates.

```
usectl orgs members list <org-id> [flags]
```

Aliases: `list, ls`


Flags:

```
  -h, --help   help for list
```

### `usectl orgs members remove`

Remove a user from the organization. The last owner cannot be removed.

```
usectl orgs members remove <org-id> <user-id> [flags]
```

Flags:

```
  -h, --help   help for remove
```

### `usectl orgs members set-role`

Change the role of an organization member.

```
usectl orgs members set-role <org-id> <user-id> [flags]
```

Flags:

```
  -h, --help          help for set-role
      --role string   Role to assign: owner, admin, member, or viewer (required)
```

### `usectl orgs projects`

Show all projects that belong to the specified organization. All organization members can view these projects.

```
usectl orgs projects <org-id> [flags]
```

Aliases: `projects, proj`


Flags:

```
  -h, --help   help for projects
```

### `usectl orgs update`

Update organization details. Only owners and admins can update.

```
usectl orgs update <org-id> [flags]
```

Flags:

```
      --desc string   New description
  -h, --help          help for update
      --name string   New organization name
```

## `usectl price`

Calculate the monthly price for a given resource configuration


### `usectl price`

Calculate the monthly price for a given resource configuration

```
usectl price [flags]
```

Flags:

```
  -h, --help               help for price
      --interval string    Billing interval: month | year (default "month")
      --ram-gb float       RAM in GiB (required)
      --storage-gb float   Storage in GiB
      --vcpu float         vCPU count (required)
```

## `usectl profile`

View your user profile (ID, username, email, role)


### `usectl profile`

View your user profile (ID, username, email, role)

```
usectl profile [flags]
usectl profile [command]
```

Flags:

```
  -h, --help   help for profile
```

### `usectl profile update`

Update one or more profile fields. Only the flags you provide will be changed.

```
usectl profile update [flags]
```

Flags:

```
      --email string      New email
  -h, --help              help for update
      --password string   New password
      --username string   New username
```

## `usectl project-billing`

View per-machine billing details and open the machine's billing portal


### `usectl project-billing`

View per-machine billing details and open the machine's billing portal

```
usectl project-billing [command]
```

Aliases: `project-billing, pbilling`


Flags:

```
  -h, --help   help for project-billing
```

### `usectl project-billing get`

Show billing status, plan, and resource allocation for a machine

```
usectl project-billing get <project-id> [flags]
```

Flags:

```
  -h, --help   help for get
```

### `usectl project-billing portal`

Open the Stripe billing portal for a machine

```
usectl project-billing portal <project-id> [flags]
```

Flags:

```
  -h, --help                help for portal
      --return-url string   Stripe portal return URL (default "https://manager.usectl.com")
```

## `usectl project-domains`

List all domains attached to a single machine


### `usectl project-domains`

List all domains attached to a single machine

```
usectl project-domains <project-id> [flags]
```

Flags:

```
  -h, --help   help for project-domains
```

## `usectl quota`

Shows the machine's CPU/RAM/storage budget vs live usage from the namespace


### `usectl quota`

Shows the machine's CPU/RAM/storage budget vs live usage from the namespace

```
usectl quota [command]
```

Flags:

```
  -h, --help   help for quota
```

### `usectl quota get`

Get quota status for a machine

```
usectl quota get <project-id> [flags]
```

Flags:

```
  -h, --help   help for get
```

### `usectl quota preview`

Dry-run a plan resize and check if the new totals fit the existing apps

```
usectl quota preview <project-id> [flags]
```

Flags:

```
  -h, --help               help for preview
      --ram-gb float       Proposed RAM in GiB (required)
      --storage-gb float   Proposed storage in GiB
      --vcpu float         Proposed vCPU total (required)
```

### `usectl quota rollover`

Restart legacy oversized pods so they come back under the per-pod default

```
usectl quota rollover <project-id> [flags]
```

Flags:

```
  -h, --help   help for rollover
```

## `usectl register`

Register a new user account. After registration, the account may need


### `usectl register`

Register a new user account. After registration, the account may need

```
usectl register [flags]
```

Flags:

```
  -h, --help   help for register
```

## `usectl registry`

Inspect the machine's container-image allowance


### `usectl registry`

Inspect the machine's container-image allowance

```
usectl registry [command]
```

Flags:

```
  -h, --help   help for registry
```

### `usectl registry usage`

How much of the machine's image allowance is used, and by which images.

```
usectl registry usage <machine-id> [flags]
```

Flags:

```
  -h, --help   help for usage
```

## `usectl stack-detect`

Returns the predicted language, framework, package manager, and other


### `usectl stack-detect`

Returns the predicted language, framework, package manager, and other

```
usectl stack-detect [flags]
```

Flags:

```
  -h, --help                  help for stack-detect
      --installation-id int   GitHub App installation ID (required)
      --owner string          Repo owner (required)
      --ref string            Git ref (branch, tag, or SHA) (default "main")
      --repo string           Repo name (required)
```

## `usectl storage`

Show S3 storage usage for a machine


### `usectl storage`

Show S3 storage usage for a machine

```
usectl storage <project-id> [flags]
```

Flags:

```
  -h, --help   help for storage
```

## `usectl trial-status`

Show current user trial status (days left)


### `usectl trial-status`

Show current user trial status (days left)

```
usectl trial-status [flags]
```

Flags:

```
  -h, --help   help for trial-status
```

## `usectl vars`

Each variable can be exposed as one of three build_target values:


### `usectl vars`

Each variable can be exposed as one of three build_target values:

```
usectl vars [command]
```

Flags:

```
  -h, --help   help for vars
```

### `usectl vars app-defaults`

Per-app overrides are nullable — pass --clear-build-target or

```
usectl vars app-defaults <project-id> <app-id> [flags]
```

Flags:

```
      --build-target string   Per-app build_target override
      --clear-build-target    Clear the per-app build_target override
      --clear-dotenv-path     Clear the per-app build .env path override
      --dotenv-auto           Auto-write .env at build time (default true)
      --dotenv-path string    Per-app build .env path override
  -h, --help                  help for app-defaults
```

### `usectl vars expose`

Set a per-variable build-target override for an app

```
usectl vars expose <project-id> <app-id> <key> [flags]
```

Flags:

```
      --build-target string   Build target: none | build_arg | env_file (default "build_arg")
  -h, --help                  help for expose
      --runtime               Inject as runtime env var too (default true)
```

### `usectl vars exposure`

Show the resolved exposure (project + app defaults + per-key overrides)

```
usectl vars exposure <project-id> <app-id> [flags]
```

Flags:

```
  -h, --help   help for exposure
```

### `usectl vars project-defaults`

Set the machine-level default build_target + .env file config

```
usectl vars project-defaults <project-id> [flags]
```

Flags:

```
      --build-target string   Default build_target: none | build_arg | env_file (default "none")
      --dotenv-auto           Auto-write .env into container working dir during build (default true)
      --dotenv-path string    Explicit relative path to write the build .env (when --dotenv-auto=false)
  -h, --help                  help for project-defaults
```

### `usectl vars unexpose`

Delete a per-variable override (revert to the resolved default)

```
usectl vars unexpose <project-id> <app-id> <key> [flags]
```

Flags:

```
  -h, --help   help for unexpose
```