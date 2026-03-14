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
curl -fsSL https://usectl.com/install.sh | bash
```

### Build from Source

Requires Go 1.25+:

```bash
git clone https://github.com/syst3mctl/usectl-cli.git
cd usectl-cli
go build -o usectl .
sudo mv usectl /usr/local/bin/
```

---

## Quick Start

```bash
# Log in
usectl login

# Connect GitHub
usectl github login

# Create a project
usectl projects create --name my-app \
  --repo https://github.com/user/repo \
  --domain my-app --port 3000

# Deploy
usectl projects deploy <id>

# View logs
usectl projects logs <id>
```

---

## Commands

### Authentication

| Command | Description |
|---|---|
| `usectl login` | Log in with email & password |
| `usectl register` | Create a new account |
| `usectl profile` | View your profile |

### Projects

| Command | Description |
|---|---|
| `usectl projects list` | List all projects |
| `usectl projects get <id>` | Show project details |
| `usectl projects create` | Create a new project |
| `usectl projects update <id>` | Update project settings |
| `usectl projects delete <id>` | Delete a project |
| `usectl projects deploy <id>` | Trigger a deployment |
| `usectl projects logs <id>` | View runtime logs |
| `usectl projects build-logs <project-id> <deployment-id>` | View build logs |
| `usectl projects status <id>` | Check container status |
| `usectl projects stats <id>` | View resource usage (CPU, memory, network) |
| `usectl projects stop <id>` | Stop a project (scale to 0) |
| `usectl projects start <id>` | Start a project (scale to 1) |

**Aliases:** `projects` → `project`, `p` · `list` → `ls`

#### Create Flags

```
--name          Project name (required)
--repo          Git repository URL (required)
--domain        Subdomain (required)
--branch        Git branch (default: main)
--type          Project type: static or service (default: service)
--port          Container port (default: 80)
--db            Provision a PostgreSQL database
--s3            Provision S3 storage (MinIO)
--addon         Add addon: database, s3, redis, nats (repeatable)
--installation-id  GitHub App installation ID
```

### Organizations

| Command | Description |
|---|---|
| `usectl orgs list` | List your organizations |
| `usectl orgs get <id>` | Get organization details |
| `usectl orgs create --name "Name"` | Create an organization |
| `usectl orgs update <id>` | Update name or description |
| `usectl orgs delete <id>` | Delete an organization |
| `usectl orgs projects <id>` | List organization projects |

**Members:**

| Command | Description |
|---|---|
| `usectl orgs members list <org-id>` | List members |
| `usectl orgs members set-role <org-id> <user-id> --role <role>` | Change role |
| `usectl orgs members remove <org-id> <user-id>` | Remove member |

**Invitations:**

| Command | Description |
|---|---|
| `usectl orgs invite list <org-id>` | List pending invitations |
| `usectl orgs invite create <org-id> --email <email>` | Invite a user |
| `usectl orgs invite info <token>` | View invitation details |
| `usectl orgs invite accept <token>` | Accept an invitation |
| `usectl orgs invite revoke <org-id> <invitation-id>` | Revoke invitation |

**Roles:** `owner`, `admin`, `member`, `viewer`

### Domains

| Command | Description |
|---|---|
| `usectl domains list` | List all domains |
| `usectl domains get <id>` | Show domain details |
| `usectl domains create <domain>` | Register a custom domain |
| `usectl domains attach <domain-id> --project <project-id>` | Attach to project |
| `usectl domains delete <id>` | Delete a domain |

### GitHub Integration

| Command | Description |
|---|---|
| `usectl github login` | Connect GitHub via OAuth |
| `usectl github installations` | List GitHub App installations |
| `usectl github repos <installation-id>` | List accessible repos |
| `usectl github branches <installation-id> <owner/repo>` | List branches |

### Admin (requires admin role)

| Command | Description |
|---|---|
| `usectl admin users list` | List all users |
| `usectl admin users enable <id>` | Enable a user |
| `usectl admin users disable <id>` | Disable a user |
| `usectl admin users set-role <id> <role>` | Set role (`user` or `admin`) |
| `usectl admin users delete <id>` | Delete a user |

### MCP (Model Context Protocol)

| Command | Description |
|---|---|
| `usectl mcp config` | Print Claude Desktop MCP configuration JSON |

---

## Global Flags

```
--api-url    Override the API base URL (default: from config or https://usectl.com)
--json       Output in JSON format (for scripting and AI agents)
--version    Show version
```

---

## Configuration

Config is stored at `~/.usectl/config.json`:

```json
{
  "token": "your-jwt-token",
  "api_url": "https://usectl.com"
}
```

Created automatically on `usectl login`.

---

## License

MIT

