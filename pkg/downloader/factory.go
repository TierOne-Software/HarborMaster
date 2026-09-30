package downloader

import (
	"fmt"
	"strings"

	"github.com/tierone/harbormaster/pkg/config"
)

// New creates a new Downloader based on the repository type.
func New(repoType config.RepositoryType, opts Options) (Downloader, error) {
	switch repoType {
	case config.RepoTypeGit:
		return NewGitDownloader(opts), nil
	case config.RepoTypeHTTP:
		return NewHTTPDownloader(opts), nil
	default:
		return nil, fmt.Errorf("unknown repository type: %s", repoType)
	}
}

// NewFromRepository creates a Downloader from a repository configuration.
func NewFromRepository(repo *config.Repository, cfg *config.Config) (Downloader, error) {
	// A defaulted (not user-set) timeout must not bound git operations: an
	// initial clone of a large repository can legitimately exceed any
	// default. HTTP keeps the default as its client timeout.
	timeout := cfg.General.Timeout
	if repo.Type == config.RepoTypeGit && !cfg.General.TimeoutExplicit {
		timeout = 0
	}

	opts := Options{
		SourceURL:     repo.URL,
		Branch:        repo.Branch,
		Tag:           repo.Tag,
		Commit:        repo.Commit,
		Depth:         repo.GetDepth(cfg.Git.CloneDepth),
		Shallow:       repo.IsShallow(cfg.Git.ShallowClone),
		Submodules:    repo.HasSubmodules(cfg.General.RecurseSubmodule),
		UserAgent:     cfg.HTTP.UserAgent,
		RetryAttempts: cfg.HTTP.RetryAttempts,
		RetryDelay:    cfg.HTTP.RetryDelay,
		Timeout:       timeout,
	}

	// For HTTP downloads the pinned "commit" is the expected SHA-256 of the
	// artifact; enforce it as a checksum so a locked sync can never replace
	// the destination with unverified content.
	if repo.Type == config.RepoTypeHTTP {
		opts.Checksum = repo.Commit
	}

	return New(repo.Type, opts)
}

// RepoNameFromURL derives a repository name from a URL: the last path
// segment with any ".git" suffix stripped. Handles SCP-style git URLs
// ("git@host:org/repo.git"), standard URLs, and plain paths. Returns ""
// when no usable name can be derived.
func RepoNameFromURL(url string) string {
	// Strip query string and fragment, then any trailing slash.
	name := url
	if i := strings.IndexAny(name, "?#"); i >= 0 {
		name = name[:i]
	}
	name = strings.TrimSuffix(name, "/")

	// Last segment, treating ':' as a separator too for SCP-style URLs
	// with no '/' after the host ("git@host:repo.git").
	if i := strings.LastIndexAny(name, "/:"); i >= 0 {
		name = name[i+1:]
	}

	name = strings.TrimSuffix(name, ".git")
	if name == "" || name == "." || name == ".." {
		return ""
	}
	return name
}

// DetectType attempts to detect the repository type from the URL.
func DetectType(url string) config.RepositoryType {
	// Unambiguous git URL schemes.
	if strings.HasPrefix(url, "git@") ||
		strings.HasPrefix(url, "git://") ||
		strings.HasPrefix(url, "ssh://") {
		return config.RepoTypeGit
	}

	// Strip query string and fragment before inspecting the path, so
	// "repo.git?token=x" is still recognized as git and "file.tar.gz?sig=y"
	// as a plain download.
	path := url
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}

	if strings.HasSuffix(path, ".git") {
		return config.RepoTypeGit
	}

	// Release assets hosted on git forges (e.g. GitHub's
	// .../releases/download/<tag>/<asset>) are plain file downloads, not
	// repositories.
	if strings.Contains(path, "/releases/download/") {
		return config.RepoTypeHTTP
	}

	// Known git hosting domains. Match against the query-stripped path so a
	// forge domain appearing in a query parameter does not misclassify a
	// plain file download.
	if strings.Contains(path, "github.com") ||
		strings.Contains(path, "gitlab.com") ||
		strings.Contains(path, "bitbucket.org") {
		return config.RepoTypeGit
	}

	// HTTP URLs
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return config.RepoTypeHTTP
	}

	// Default to git
	return config.RepoTypeGit
}
