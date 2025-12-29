package config

// RepositoryType defines the type of repository.
type RepositoryType string

const (
	RepoTypeGit  RepositoryType = "git"
	RepoTypeHTTP RepositoryType = "http"
)

// Repository represents a single repository definition.
type Repository struct {
	Name       string
	URL        string
	Type       RepositoryType
	Path       string   // Local path relative to work_dir
	Branch     string   // Git branch (optional)
	Tag        string   // Git tag (optional)
	Commit     string   // Git commit SHA (optional)
	Shallow    *bool    // Override global shallow clone setting
	Depth      *int     // Override global clone depth
	Submodules *bool    // Override global submodule setting
	Tags       []string // User-defined tags for filtering
}

// RepositoryFile is the raw TOML structure for a repository.
type RepositoryFile struct {
	Name       string   `toml:"name"`
	URL        string   `toml:"url"`
	Type       string   `toml:"type"`
	Path       string   `toml:"path,omitempty"`
	Branch     string   `toml:"branch,omitempty"`
	Tag        string   `toml:"tag,omitempty"`
	Commit     string   `toml:"commit,omitempty"`
	Shallow    *bool    `toml:"shallow,omitempty"`
	Depth      *int     `toml:"depth,omitempty"`
	Submodules *bool    `toml:"submodules,omitempty"`
	Tags       []string `toml:"tags,omitempty"`
}

// GetEffectiveRef returns the reference (branch, tag, or commit) to checkout.
// Priority: commit > tag > branch > default.
func (r *Repository) GetEffectiveRef(defaultBranch string) string {
	if r.Commit != "" {
		return r.Commit
	}
	if r.Tag != "" {
		return r.Tag
	}
	if r.Branch != "" {
		return r.Branch
	}
	return defaultBranch
}

// GetEffectivePath returns the local path for the repository.
// Defaults to the repository name if not specified.
func (r *Repository) GetEffectivePath() string {
	if r.Path != "" {
		return r.Path
	}
	return r.Name
}

// IsShallow returns whether to use shallow clone for this repository.
func (r *Repository) IsShallow(defaultShallow bool) bool {
	if r.Shallow != nil {
		return *r.Shallow
	}
	return defaultShallow
}

// GetDepth returns the clone depth for this repository.
func (r *Repository) GetDepth(defaultDepth int) int {
	if r.Depth != nil {
		return *r.Depth
	}
	return defaultDepth
}

// HasSubmodules returns whether to recurse submodules for this repository.
func (r *Repository) HasSubmodules(defaultSubmodules bool) bool {
	if r.Submodules != nil {
		return *r.Submodules
	}
	return defaultSubmodules
}
