# Harbormaster Design Document

## Overview

Harbormaster is a command-line tool designed to manage and synchronize remote repositories and libraries, similar to Google's `repo` or Fuchsia's `jiri`. The tool uses a nautical theme throughout its design and implementation, where repositories are referred to as "vessels" and collections of repositories are called "fleets."

## Architecture

### Core Principles

1. **Separation of Concerns**: Clear boundaries between operational logic, UI, and coordination
2. **Modularity**: Each component can be developed, tested, and maintained independently
3. **Concurrent Operations**: Support for parallel downloads with progress reporting
4. **Configuration-Driven**: TOML-based configuration for defining repositories and projects

### Component Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   CLI Frontend  │────│ Repository Mgr  │────│   Downloaders   │
│   (cmd/main.go) │    │                 │    │  (git, http)    │
└─────────────────┘    └─────────────────┘    └─────────────────┘
                              │
                              │
                       ┌─────────────────┐
                       │   UI Manager    │
                       │  (Bubbletea)    │
                       └─────────────────┘
                              │
                       ┌─────────────────┐
                       │    Progress     │
                       │     Types       │
                       └─────────────────┘
```

## Package Structure

### `pkg/config`
Handles configuration file parsing and management using TOML format.

**Key Components:**
- `Config`: Main configuration structure with parsed values
- `ConfigFile`: Raw configuration structure for file I/O
- `Repository`: Individual repository definition
- `Project`: Collection of repositories (fleet)

**Features:**
- Environment variable expansion
- Path expansion (~ to home directory)
- Default value handling
- Validation and type conversion

### `pkg/downloader`
Provides pluggable downloader implementations for different repository types.

**Interface Design:**
```go
type Downloader interface {
    Download(source, destination string) (string, error)
    DownloadWithProgress(source, destination string) (string, chan ProgressUpdate, error)
    GetInfo() map[string]string
}
```

**Implementations:**
- `HTTPDownloader`: Downloads files via HTTP/HTTPS with progress reporting
- `GitDownloader`: Clones and updates Git repositories with options

**Key Features:**
- Async progress reporting via channels
- Error handling with detailed context
- Configurable options (depth, branch, submodules)
- No UI dependencies

### `pkg/types`
Defines common data structures for inter-component communication.

**Key Types:**
- `ProgressUpdate`: Internal progress reporting between downloader and manager
- `ProgressMsg`: Rich progress information for UI display

### `pkg/ui`
Manages the terminal user interface using Bubbletea framework.

**Components:**
- `ProgressManager`: Coordinates UI display and collects results
- `progressModel`: Bubbletea model for rendering progress
- `operationState`: Tracks individual operation states

**Features:**
- Real-time progress bars and spinners
- Error display with styling
- Concurrent operation tracking
- Keyboard shortcuts (quit with 'q')

### `pkg/manager`
Orchestrates the entire synchronization process.

**Key Component:**
- `RepositoryManager`: Central coordinator for all operations

**Responsibilities:**
- Repository type detection and routing
- Progress message transformation
- Concurrent operation management
- Error collection and reporting

## Configuration Format

Harbormaster uses TOML configuration files with the following structure:

```toml
[general]
work_dir = "~/projects/workspace"
cache_dir = "~/projects/.cache" 
timeout = "5m"
default_branch = "main"
recurse_submodule = true

[http]
user_agent = "Harbormaster/1.0"
retry_attempts = 3
retry_delay = "2s"

[git]
shallow_clone = true
clone_depth = 1

[[repository]]
name = "example-repo"
url = "https://github.com/user/repo.git"
type = "git"
branch = "main"
tags = ["tag1", "tag2"]

[[project]]
name = "example-fleet"
repositories = ["example-repo"]
tags = ["fleet-tag"]
```

## Data Flow

### Synchronization Process

1. **Configuration Loading**: Parse TOML file and apply defaults
2. **Repository Selection**: Based on CLI arguments (all, project, or single repo)
3. **Parallel Execution**: Start goroutines for each repository
4. **Progress Coordination**: UI manager collects progress from all operations
5. **Result Collection**: Gather final status and errors
6. **Display Results**: Show success/failure summary

### Progress Reporting Flow

```
Downloader → ProgressUpdate → Repository Manager → ProgressMsg → UI Manager → Display
```

1. **Downloader** emits `ProgressUpdate` via channel
2. **Repository Manager** transforms to `ProgressMsg` with context
3. **UI Manager** receives and displays progress
4. **Bubbletea** renders real-time terminal UI

## Error Handling Strategy

### Levels of Error Handling

1. **Operation Level**: Individual download failures
2. **Repository Level**: Repository-specific configuration issues  
3. **Project Level**: Missing repositories in projects
4. **System Level**: Configuration, file system, or network issues

### Error Reporting

- Errors are captured at each level and wrapped with context
- UI displays errors with clear styling and messages
- Final summary shows all failures with details
- Non-fatal errors don't stop other operations

## Concurrency Design

### Thread Safety

- Repository Manager uses mutex for shared state
- Each downloader operation runs in separate goroutine
- Progress channels provide safe communication
- UI updates are serialized through Bubbletea's message system

### Resource Management

- Bounded goroutines (one per repository)
- Channel cleanup to prevent leaks
- Timeout handling for HTTP downloads (configurable via `general.timeout`)

## Extensibility Points

### Adding New Downloader Types

1. Implement the `Downloader` interface
2. Add type detection in Repository Manager
3. Update configuration schema if needed
4. No UI changes required

### UI Alternatives

- Replace UI Manager while keeping same channel interface
- Progress types provide all necessary information
- Could add web UI, JSON output, or silent mode

### Configuration Extensions

- Add new fields to config structures
- Extend TOML parsing with validation
- Maintain backward compatibility

## Performance Considerations

### Optimization Strategies

1. **Parallel Downloads**: Multiple repositories sync simultaneously
2. **Shallow Clones**: Git repositories use minimal depth by default
3. **Progress Streaming**: Real-time updates without blocking operations
4. **Efficient Channels**: Bounded channels prevent memory buildup

### Scalability

- Memory usage scales with number of concurrent operations
- Network bandwidth is primary bottleneck
- UI performance independent of operation count
- Configuration parsing is one-time cost

## Security Considerations

### Input Validation

- URL validation for repository sources
- Validation of repository paths against directory traversal
- Configuration file permission checks
- Environment variable expansion limits

### Network Security

- HTTPS supported for downloads (plain `http://` URLs are accepted; prefer HTTPS)
- Git credential handling via system Git
- Proxy support for corporate environments
- User-Agent identification for requests

## Testing Strategy

### Unit Testing

- **Downloaders**: Mock HTTP servers and Git repositories
- **Configuration**: Various TOML scenarios and edge cases
- **UI Components**: Isolated Bubbletea model testing
- **Progress Types**: Message transformation validation

### Integration Testing

- End-to-end repository synchronization
- Configuration loading and validation
- Error handling across component boundaries
- Concurrent operation coordination

### Manual Testing

- Real repository synchronization
- UI responsiveness and display quality
- Error scenarios and recovery
- Performance with large repositories

## Future Enhancements

### Planned Features

1. **Differential Updates**: Only download changed files
2. **Caching Layer**: Local cache for frequently accessed repositories
3. **Webhook Support**: Trigger syncs on repository changes
4. **Plugin System**: Custom downloader implementations
5. **Configuration Templates**: Predefined setups for common use cases

### Potential Improvements

- **Web Interface**: Browser-based management console
- **Monitoring**: Metrics collection and alerting
- **Cloud Storage**: Support for S3, GCS, and other cloud providers

## Deployment

### Build Process

```bash
go build -o harbormaster ./cmd/harbormaster
```

### Installation

- Single binary deployment
- Configuration file discovery (current working directory)
- Optional systemd service for scheduled syncs
- Docker container for isolated environments

### Configuration Management

- Version control for configuration files
- Environment-specific configurations
- Configuration validation tools
- Migration utilities for format changes