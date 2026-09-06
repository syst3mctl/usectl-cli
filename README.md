# usectl

**Command-line interface for the [usectl.com](https://usectl.com) platform.**

Manage projects, deployments, organizations, domains, and more from the terminal.

## Installation

### Homebrew (macOS & Linux)

```bash
brew install --cask syst3mctl/usectl-cli/usectl
```

### Snap Store (Ubuntu & Linux)

```bash
snap install usectl
```

### AUR (Arch Linux)

```bash
yay -S usectl
```

### Quick Install Script

```bash
curl -fsSL https://manager.usectl.com/install.sh | bash
```

### Manual Install (Windows, Linux, macOS)

Download the appropriate binary from [GitHub Releases](https://github.com/syst3mctl/usectl-cli/releases).

**Windows:**
1. Download the `usectl_vX.X.X_windows_amd64.zip` asset.
2. Extract the `.zip` archive.
3. Move `usectl.exe` into a safe directory and add that directory to your Windows System PATH.

### Build from Source

Requires Go 1.25+:

```bash
git clone https://github.com/syst3mctl/usectl-cli.git
cd usectl-cli
go build -o usectl .
sudo mv usectl /usr/local/bin/
```


---

## Mental model

Two nouns, and keeping them apart explains most of the CLI:

- A **machine** is a Kubernetes namespace plus a resource wallet — vCPU, RAM,
  storage, one subscription. It holds no code.
- A **pod** is one workload inside a machine. Repository and branch (or a
  prebuilt image), domain, ports, visibility, container limits and rollout
  strategy all belong to the pod. A machine with three pods has three repos.

**Addons** (Postgres, Redis, NATS, S3, …) are provisioned per machine, but a pod
only receives their credentials once the addon is **attached** to it. A machine
owning a database that a given pod cannot see is the usual reason a pod starts
but cannot connect to anything.

---

## Quick start

```bash
usectl login                       # opens your browser
usectl github login                # connect GitHub for private repos

# 1. a machine: a namespace and a quota
usectl machines create my-app --vcpu 2 --ram 4 --storage 10

# 2. a pod inside it
usectl machines pods create my-app web \
  --repo https://github.com/user/repo --port 3000 --addon database

# 3. deploy and watch
usectl machines deploy my-app
usectl machines pods my-app
usectl machines logs my-app web -f
```

Run `usectl machines create` or `usectl machines pods create` with no flags on a
terminal to be prompted for each value instead.

---

## Naming, context and argument order

Machines, pods and addons are all addressable **by name** — UUIDs are never
required:

```bash
usectl machines pods my-app
usectl machines logs my-app web
usectl machines addons get my-app database/analytics
```

Set a default so the machine can be omitted entirely:

```bash
usectl use my-app web          # machine + pod
usectl machines pods           # acts on my-app
usectl machines logs -f        # tails my-app/web
usectl use --clear
```

Precedence, highest first: an explicit argument, then `-m/--machine`, then
`$USECTL_MACHINE` / `$USECTL_POD`, then `usectl use`. Whenever a target is
resolved from anything but an explicit argument, the source is echoed to
**stderr** so an implicit target is never invisible:

```
→ machine my-app (from usectl use)
```

Where a command takes both a machine and a pod (or addon), **the order does not
matter** — the two names resolve against different collections, so only one
reading of the pair can be valid.

### Working inside one machine

`usectl machines enter` opens an interactive sub-shell bound to a single
machine, so a session of related work does not repeat the machine on every line:

```console
$ usectl machines enter my-app
Scoped to machine "my-app". 'help' for commands, 'exit' to leave.

usectl(my-app)> pods
usectl(my-app)> pods set web port=8080
usectl(my-app)> addons get database
usectl(my-app)> logs web -f
usectl(my-app)> exit
$
```

- The machine is supplied for you, so `pods` means `machines pods my-app`.
- Commands are looked up under `machines` first, then at the top level — so
  `billing`, `github` and `orgs` still work inside the shell.
- `help` lists the available commands.
- **Leave with `exit`, `quit`, or Ctrl-D.** Ctrl-C interrupts the running
  command, not the shell.
- A failed command prints its error and returns you to the prompt; it does not
  end the session.
- Flags do not carry over between lines — a `--json` or `--reveal` on one line
  is not still in force on the next.

Line editing is basic: there is no history or arrow-key recall. `enter` is a
convenience for people, and is not scriptable — automation should pass the
machine explicitly or use `-m`.

If you only want to avoid retyping the machine across separate commands, use
`usectl use` instead; it persists between terminal sessions, which `enter` does
not.

---

## Commands

### Authentication

| Command | Description |
|---|---|
| `usectl login` | Browser login — approve in the dashboard, no password typed |
| `usectl login --password` | Email/password prompt (headless servers, CI) |
| `usectl logout` | Discard the saved credentials |
| `usectl register` / `usectl profile` | Create an account / view your profile |

### Machines

| Command | Description |
|---|---|
| `usectl machines list` | Machines with status, size and billing |
| `usectl machines get <machine>` | Details, sizing, billing, recent deployments |
| `usectl machines create <name> --vcpu N --ram N --storage N` | Create a machine |
| `usectl machines settings <machine> [key=value]` | Show or change machine settings |
| `usectl machines usage <machine>` | Allocation against the plan's limits |
| `usectl machines delete <machine>` | Delete a machine (confirmation required) |
| `usectl machines deploy <machine> [pod]` | Build and deploy |
| `usectl machines deployments <machine> [pod]` | Deployment history |
| `usectl machines build-logs <machine> <deployment>` | Build logs (short ids accepted) |
| `usectl machines rollback <machine> <deployment>` | Redeploy a previous image |
| `usectl machines logs <machine> [pod] [-f]` | Runtime logs |
| `usectl machines shell <machine>` | Interactive shell in a running pod |
| `usectl machines enter <machine>` | Sub-shell scoped to one machine |

Sizes accept a bare number of GB or a suffix: `--ram 4`, `--ram 4gb` and
`--ram 4096mb` are the same machine.

**Aliases:** `machines` → `machine`, `m`, `projects`, `project`, `p`

### Pods

| Command | Description |
|---|---|
| `usectl machines pods <machine>` | Every pod: source, ports, limits, rollout, addons, nodes |
| `usectl machines pods create <machine> <name>` | Add a pod (`--repo` or `--image`) |
| `usectl machines pods set <machine> <pod> [key=value]` | Show or change pod config |
| `usectl machines pods delete <machine> <pod>` | Remove a pod |
| `usectl machines pods open-port <machine> <pod> <port>[/proto] [name]` | Open an internal port |
| `usectl machines pods close-port <machine> <pod> <port>` | Close one |
| `usectl machines pods env <machine> <pod> [KEY=value]` | The pod's full environment |
| `usectl machines pods addons <machine> <pod>` | Which addons feed this pod |
| `usectl machines pods attach-addon <machine> <pod> <addon>…` | Inject an addon's credentials |
| `usectl machines pods logs\|stats\|restart\|shell\|diagnostics` | Runtime operations |

`pods set` with no `key=value` prints every settable key with its current value.

#### A pod needs a domain to be reachable

Creating a pod does **not** publish it. Without a domain it gets a ClusterIP
Service and no IngressRoute — reachable from sibling pods in the same machine by
name, and from nothing else. A browser, a webhook, another machine, or a
frontend calling an API all need a domain:

```bash
usectl machines pods create api web --repo <url> --port 8080 --domain api
usectl machines pods set api web domain=api.example.com
```

No dot → a platform subdomain (`api` → `api.usectl.com`). With a dot → your own
domain, whose DNS must point at the platform.

Omit it only for a genuinely internal pod — a worker, a queue consumer, a
backend called by a sibling — and pass `--private` to say so. `machines pods`
shows this per pod: `public → host` when reachable, `public (no domain
attached)` when nothing can call it.

#### Pods from a prebuilt image — no GitHub involved

```bash
usectl machines pods create my-app cache --image redis:7 --port 6379 --private
usectl machines pods create my-app web  --image ghcr.io/acme/api:v1.2.3 --port 8080 \
  --registry-user acme-bot --registry-password "$GHCR_TOKEN"
```

No repository, no branch, no GitHub App, no build — the reference is deployed
as-is. `--repo` and `--image` are mutually exclusive.

### Addons

| Command | Description |
|---|---|
| `usectl machines addons list <machine>` | Addons with size, version, backups, UI |
| `usectl machines addons get <machine> <addon>` | Config, credentials, backups, pods |
| `usectl machines addons add <machine>` | Interactive: pick from the catalogue |
| `usectl machines addons add <machine> --type database --mode dedicated` | Non-interactive |
| `usectl machines addons remove <machine> <addon>` | Deprovision (`delete` also works) |
| `usectl machines addons start\|stop <machine> <addon>` | Scale a dedicated addon |

An addon is named by instance name, type, or `type/name` when a bare name would
be ambiguous (a machine can hold both a `primary` database and a `primary`
bucket).

Secrets are **masked by default** in `addons get` and the env listings; pass
`--reveal` to show them. `--json` is never masked, since automation needs the
real value.

### Environment variables

| Command | Description |
|---|---|
| `usectl machines envs <machine>` | Machine-wide variables |
| `usectl machines envs <machine> <pod>` | Everything that pod receives, with sources |
| `usectl machines envs <machine> [pod] KEY=value` | Set |
| `usectl machines envs delete <machine> KEY…` | Remove |
| `usectl machines envs protect <machine> [pod] protect\|open KEY…` | Write-only, or readable again |

The pod view is the merged one: machine-wide values, the pod's own overrides and
attached addon credentials, each tagged with where it came from.

Never set `DATABASE_URL`, `REDIS_URL` and the like by hand — they are injected
from the addon and would be overwritten.

### Groups, members, quota, domains, cron

| Command | Description |
|---|---|
| `usectl machines groups list\|create\|delete <machine>` | Isolated namespaces within a machine |
| `usectl machines pods set <machine> <pod> group=stage\|none` | Move a pod between groups |
| `usectl machines groups move-addon <machine> <addon> <group>` | Move an addon (`--pvc leave\|destroy`) |
| `usectl machines members <machine>` | Members, roles, invitations |
| `usectl machines quota <machine>` | Quota, recommendations, resize previews |
| `usectl machines domains <machine>` | Custom domains |
| `usectl machines cron <machine>` | Scheduled jobs and run history |

### Organizations, domains, GitHub, admin

| Command | Description |
|---|---|
| `usectl orgs …` | Organizations, members, invitations |
| `usectl domains …` | Register, attach and verify domains |
| `usectl github login\|installations\|repos\|branches` | GitHub App integration |
| `usectl admin users …` | Admin-only user management |
| `usectl mcp config` | MCP configuration for AI assistants |

---

## Scripting and AI agents

```bash
usectl schema --json      # the entire command tree: args, flags, aliases
usectl schema             # the same tree as a readable outline
```

One call replaces recursively scraping `--help`. The document is generated from
the live command tree, so it always matches the binary that produced it, and it
carries notes on target resolution, destructive commands and the machine/pod
model.

Every command accepts `--json`. Commands that would otherwise prompt never do so
under `--json`, `--yes`, or a non-terminal stdin — they exit non-zero listing
exactly which flags were missing, so an agent is never blocked on an invisible
prompt:

```json
{"error":"missing_required","missing":["vcpu","ram"]}
```

Colour is written only to a terminal: piped output and `--json` carry no escape
sequences, `NO_COLOR` is honoured, and `--color=auto|always|never` overrides.

---

## Global flags

```
-m, --machine   Machine to act on (overrides $USECTL_MACHINE and 'usectl use')
    --api-url   API base URL (default: from config or https://manager.usectl.com)
    --json      JSON output, for scripting and AI agents
    --color     auto | always | never
    --version   Show version
```

---

## Configuration

`~/.usectl/config.json`, created by `usectl login`:

```json
{
  "token": "...",
  "refresh_token": "...",
  "api_url": "https://manager.usectl.com",
  "machine": "my-app",
  "pod": "web"
}
```

The access token is refreshed transparently; the rotated refresh token is
re-saved on every use.

---

## License

MIT

