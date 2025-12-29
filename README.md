# Harbormaster

A command-line tool for managing and synchronizing multiple repositories.

## Installation

```bash
go install github.com/TierOne-Software/HarborMaster/cmd/harbormaster@latest
```

## Quick Start

```bash
# Initialize a new workspace
hm init

# Add a repository
hm add https://github.com/user/repo.git --name my-repo

# Sync all repositories
hm sync

# Check status
hm status
```

## Commands

### init

Initialize a new Harbormaster workspace.

```bash
hm init [flags]
```

| Flag | Description |
|------|-------------|
| `-f, --force` | Overwrite existing configuration |
| `--example` | Include example repository entries |

### add

Add a repository to the configuration.

```bash
hm add <url> [flags]
```

| Flag | Description |
|------|-------------|
| `-n, --name` | Repository name (required) |
| `-t, --type` | Repository type: `git` or `http` (auto-detected) |
| `-b, --branch` | Git branch to track |
| `--tag` | Git tag to track |
| `--commit` | Git commit SHA to pin |
| `-p, --path` | Local path (relative to work_dir) |
| `--sync` | Sync immediately after adding |
| `--tags` | Tags for filtering (comma-separated) |

### remove

Remove a repository from the configuration.

```bash
hm remove <repository> [flags]
hm rm <repository> [flags]
```

| Flag | Description |
|------|-------------|
| `--delete-files` | Also delete local repository files |
| `-f, --force` | Don't prompt for confirmation |

### sync

Synchronize repositories.

```bash
hm sync [repository...] [flags]
```

| Flag | Description |
|------|-------------|
| `--locked` | Sync to exact commits in lock file |
| `-p, --project` | Sync repositories in a project |
| `-t, --tag` | Sync repositories with a tag |
| `--parallel` | Concurrent operations (default: 4) |
| `--dry-run` | Show what would be synced |

### status

Show repository status.

```bash
hm status [repository...] [flags]
```

| Flag | Description |
|------|-------------|
| `--json` | Output as JSON |
| `-p, --project` | Show status for project only |
| `--porcelain` | Machine-readable output |

**Status values:**
- `ok` - Repository is synced and clean
- `missing` - Repository doesn't exist locally
- `dirty` - Repository has uncommitted changes
- `outdated` - Repository differs from lock file

**Lock status:**
- `locked` - Current commit matches lock file
- `drift` - Current commit differs from lock file
- `-` - No lock file entry

### list

List repositories, projects, and tags.

```bash
hm list repos [flags]      # List repositories
hm list projects [flags]   # List projects
hm list tags [flags]       # List all tags
```

| Flag | Description |
|------|-------------|
| `--json` | Output as JSON |
| `-p, --project` | Filter by project |
| `-t, --tag` | Filter by tag |

### project

Manage projects (repository groups).

```bash
hm project add <name> [flags]              # Create a project
hm project remove <name> [flags]           # Remove a project
hm project add-repo <project> <repo>       # Add repo to project
hm project remove-repo <project> <repo>    # Remove repo from project
```

**project add flags:**

| Flag | Description |
|------|-------------|
| `-r, --repos` | Initial repositories (comma-separated) |
| `-t, --tags` | Project tags |

**project remove flags:**

| Flag | Description |
|------|-------------|
| `-f, --force` | Don't prompt for confirmation |

## Global Flags

| Flag | Description |
|------|-------------|
| `-c, --config` | Config file path |
| `-w, --work-dir` | Override work directory |
| `-q, --quiet` | Minimal output |
| `--no-color` | Disable colored output |

## Configuration

Harbormaster uses a TOML configuration file (`.harbormaster.toml`):

```toml
[general]
work_dir = "~/projects"
timeout = "10m"
default_branch = "main"

[git]
shallow_clone = true
clone_depth = 1

[http]
user_agent = "Harbormaster/1.0"
retry_attempts = 3

[[repository]]
name = "my-app"
url = "https://github.com/user/my-app.git"
type = "git"
branch = "main"
path = "my-app"
tags = ["frontend"]

[[repository]]
name = "api"
url = "https://github.com/user/api.git"
type = "git"
branch = "develop"
tags = ["backend"]

[[project]]
name = "web-stack"
repositories = ["my-app", "api"]
tags = ["production"]
```

## Lock File

Harbormaster maintains a lock file (`.harbormaster.lock`) that records exact commit SHAs for reproducible syncs. Use `hm sync --locked` to sync to the locked state.

## Examples

```bash
# Add a git repository tracking a specific branch
hm add https://github.com/user/repo.git -n repo -b develop

# Add and immediately sync
hm add https://github.com/user/repo.git -n repo --sync

# Pin to a specific commit
hm add https://github.com/user/repo.git -n repo --commit abc123

# Sync a specific project
hm sync -p my-project

# Sync repositories with a specific tag
hm sync -t backend

# Check what would be synced
hm sync --dry-run

# Reproducible sync using lock file
hm sync --locked

# Create a project with initial repositories
hm project add backend --repos=api,database --tags=production

# Get status as JSON
hm status --json
```
