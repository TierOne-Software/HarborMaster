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
		Timeout:       cfg.General.Timeout,
	}

	return New(repo.Type, opts)
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

	// Known git hosting domains.
	if strings.Contains(url, "github.com") ||
		strings.Contains(url, "gitlab.com") ||
		strings.Contains(url, "bitbucket.org") {
		return config.RepoTypeGit
	}

	// HTTP URLs
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return config.RepoTypeHTTP
	}

	// Default to git
	return config.RepoTypeGit
}
