# usectl

**Command-line interface for the K-Deploy platform.**

Manage projects, deployments, domains, and more on your K-Deploy cluster from the terminal.

---

## Installation

### Quick Install (Linux)

```bash
curl -fsSL https://usectl.com/install.sh | bash
```

This downloads the latest `usectl` binary to `/usr/local/bin`.

### Manual Install

Download the binary from [releases](https://usectl.com/releases), make it executable, and move it to your PATH:

```bash
curl -fsSL -o usectl https://usectl.com/releases/latest/usectl-linux-amd64
chmod +x usectl
sudo mv usectl /usr/local/bin/
```

### Build from Source

Requires Go 1.25+:

```bash
git clone https://github.com/giorgi/usectl.git
cd usectl
go build -o usectl .
sudo mv usectl /usr/local/bin/
```

---

## Quick Start

```bash
# Create an account
usectl register

# Or log in to an existing one
usectl login

# List your projects
usectl projects list

# Deploy a project
usectl projects deploy <project-id>
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

**Aliases:** `projects` → `project`, `p`  
**Aliases:** `list` → `ls`

#### Create Flags

```
--name        Project name
--repo        Git repository URL
--branch      Git branch (default: master)
--domain      Subdomain (e.g. "myapp" → myapp.usectl.com)
--type        Project type: static or service
--port        Container port (for service type)
--db          Provision a PostgreSQL database
--s3          Provision S3 storage
--gh-token    GitHub personal access token (for private repos)
```

### Domains

| Command | Description |
|---|---|
| `usectl domains list` | List all domains |
| `usectl domains get <id>` | Show domain details |
| `usectl domains create <domain>` | Register a custom domain |
| `usectl domains attach <domain-id> --project <project-id>` | Attach domain to project |
| `usectl domains delete <id>` | Delete a domain |

**Aliases:** `domains` → `domain`, `d`

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

Paste the output into your Claude Desktop `claude_desktop_config.json` to connect Claude to your K-Deploy cluster.

---

## Global Flags

```
--api-url    Override the API base URL (default: https://usectl.com)
--json       Output in JSON format
```

---

## Configuration

Config is stored at `~/.usectl/config.json` and contains:

```json
{
  "token": "your-jwt-token",
  "api_url": "https://usectl.com"
}
```

This file is created automatically on `usectl login`.

---

## Examples

```bash
# Create a static site from a GitHub repo
usectl projects create \
  --name "my-site" \
  --repo "https://github.com/user/my-site.git" \
  --domain "my-site" \
  --type static

# Create a service with a database
usectl projects create \
  --name "my-api" \
  --repo "https://github.com/user/my-api.git" \
  --domain "api" \
  --type service \
  --port 8080 \
  --db

# Check logs of a running project
usectl projects logs 97b4acce --lines 100

# Get JSON output for scripting
usectl projects list --json | jq '.[].name'

# Connect to a self-hosted cluster
usectl login --api-url https://my-cluster.example.com
```

---

## License

MIT
