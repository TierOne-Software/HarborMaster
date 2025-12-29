package downloader

import (
	"time"

	"github.com/tierone/harbormaster/pkg/types"
)

// Downloader defines the interface for downloading/syncing repositories.
type Downloader interface {
	// Download performs a blocking download operation.
	// Returns the resolved reference (commit SHA for git, content hash for HTTP).
	Download(source, destination string) (string, error)

	// DownloadWithProgress performs download with progress reporting.
	// Returns: resolved reference, progress channel, error.
	// The channel is closed when the operation completes.
	DownloadWithProgress(source, destination string) (string, <-chan types.ProgressUpdate, error)

	// Update updates an existing repository/file.
	// Returns the resolved reference.
	Update(destination string) (string, error)

	// UpdateWithProgress updates with progress reporting.
	UpdateWithProgress(destination string) (string, <-chan types.ProgressUpdate, error)

	// GetCurrentRef returns the current reference at destination.
	// For git: HEAD commit SHA. For HTTP: content hash.
	GetCurrentRef(destination string) (string, error)

	// Type returns the downloader type (git, http).
	Type() string
}

// Options configures downloader behavior.
type Options struct {
	// Git-specific options
	Branch     string
	Tag        string
	Commit     string
	Depth      int
	Shallow    bool
	Submodules bool

	// HTTP-specific options
	UserAgent     string
	RetryAttempts int
	RetryDelay    time.Duration

	// Common options
	Timeout time.Duration
}

// DefaultOptions returns options with default values.
func DefaultOptions() Options {
	return Options{
		Depth:         1,
		Shallow:       true,
		Submodules:    true,
		UserAgent:     "Harbormaster/1.0",
		RetryAttempts: 3,
		RetryDelay:    2 * time.Second,
		Timeout:       10 * time.Minute,
	}
}

// GetEffectiveRef returns the ref to checkout (commit > tag > branch).
func (o *Options) GetEffectiveRef() string {
	if o.Commit != "" {
		return o.Commit
	}
	if o.Tag != "" {
		return o.Tag
	}
	return o.Branch
}
