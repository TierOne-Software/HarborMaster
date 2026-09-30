# Harbormaster

A command-line tool for managing and synchronizing multiple repositories.

## Installation

Build from source:

```bash
git clone https://github.com/tierone/harbormaster.git
cd harbormaster
make build
```

This produces the `hm` binary in the repository root; copy it somewhere on your `PATH` (e.g. `install -m 0755 hm ~/.local/bin/`).

Alternatively, from a clone you can run `go install ./cmd/harbormaster` (or `make install`) — note this installs the binary under the name `harbormaster`, not `hm`.

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
| `-n, --name` | Repository name (default: derived from the URL) |
| `--type` | Repository type: `git` or `http` (auto-detected) |
| `-b, --branch` | Git branch to track |
| `--tag` | Git tag to track |
| `--commit` | Git commit SHA to pin |
| `-p, --path` | Local path (relative to work_dir) |
| `--sync` | Sync immediately after adding |
| `--tags` | Tags for filtering (comma-separated) |
| `--project` | Existing project(s) to add the repository to (comma-separated) |

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

### lock

Update the lock file without touching repository checkouts.

```bash
hm lock update [repository...] [flags]   # Pin to latest commit on the configured branch
hm lock adopt [repository...] [flags]    # Pin to the current local HEAD
```

`update` resolves each repository's configured branch against its remote
(`git ls-remote`) and records the tip commit. Repositories whose config pins
a commit or tag are skipped.

`adopt` records each repository's current local HEAD — for example after
committing and pushing from within a checkout. A HEAD that has not been
pushed to origin cannot be reproduced by `hm sync --locked` elsewhere, so it
is refused unless `--force` is given.

| Flag | Description |
|------|-------------|
| `-p, --project` | Apply to repositories in a project |
| `-t, --tag` | Apply to repositories with a tag |
| `--dry-run` | Show what would change without writing the lock file |
| `--sync` | (`update` only) Also check out the new pins afterwards |
| `--force` | (`adopt` only) Pin a HEAD that has not been pushed to origin |

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

### work

Manage coordinated multi-repository work sessions. Create a shared branch across
selected repositories, then commit, push, and create pull requests across all of
them in a single command.

```bash
hm work start <branch> [repository...] [flags]   # Start a work session
hm work end [flags]                               # End session, restore branches
hm work add <repository>                          # Add repo to session
hm work remove <repository>                       # Remove repo from session
hm work status [flags]                            # Show session status
hm work commit [repository...] [flags]            # Commit across repos
hm work push [repository...] [flags]              # Push across repos
hm work pr [repository...] [flags]                # Create PRs across repos
```

**work start flags:**

| Flag | Description |
|------|-------------|
| `-p, --project` | Include repositories from a project |
| `-t, --tag` | Include repositories with a tag |

**work end flags:**

| Flag | Description |
|------|-------------|
| `-f, --force` | End even with uncommitted changes |

**work status flags:**

| Flag | Description |
|------|-------------|
| `--json` | Output as JSON |

**work commit flags:**

| Flag | Description |
|------|-------------|
| `-m, --message` | Commit message (required) |
| `--all` | Commit in all session repositories |

**work push flags:**

| Flag | Description |
|------|-------------|
| `--all` | Push all session repositories |

**work pr flags:**

| Flag | Description |
|------|-------------|
| `--title` | Pull request title (defaults to branch name) |
| `--body` | Pull request body |
| `--all` | Create PRs for all session repositories |

> **Note:** `hm work pr` requires the [GitHub CLI](https://cli.github.com/) (`gh`) to be installed and authenticated.

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

Update the pins with `hm lock update` (latest commit on each configured branch) or `hm lock adopt` (current local HEAD); neither touches your checkouts. A plain `hm sync` also refreshes the lock file as a side effect of syncing.

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

# Bump a pin to the latest commit on its branch (checkout untouched)
hm lock update cports

# Record the local HEAD you just committed and pushed
hm lock adopt cragutils

# Create a project with initial repositories
hm project add backend --repos=api,database --tags=production

# Get status as JSON
hm status --json

# Start a work session across a project
hm work start feature/auth -p backend

# Check changes across all repos in the session
hm work status

# Commit and push all repos at once
hm work commit --all -m "add auth middleware"
hm work push --all

# Create PRs for all repos
hm work pr --all --title "Add auth middleware"

# End the session
hm work end
```
