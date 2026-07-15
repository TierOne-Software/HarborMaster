package config

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// ValidationError represents a configuration validation error.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidateConfig validates the entire configuration.
func ValidateConfig(cfg *Config) error {
	if err := validateGeneral(cfg); err != nil {
		return err
	}

	// Validate repositories
	repoNames := make(map[string]bool)
	repoPaths := make(map[string]string) // cleaned effective path -> repo name
	for i, repo := range cfg.Repositories {
		if err := validateRepository(&repo, i); err != nil {
			return err
		}
		if repoNames[repo.Name] {
			return &ValidationError{
				Field:   fmt.Sprintf("repository[%d].name", i),
				Message: fmt.Sprintf("duplicate repository name: %s", repo.Name),
			}
		}
		repoNames[repo.Name] = true

		effectivePath := filepath.Clean(repo.GetEffectivePath())
		if other, exists := repoPaths[effectivePath]; exists {
			return &ValidationError{
				Field:   fmt.Sprintf("repository[%d].path", i),
				Message: fmt.Sprintf("path %q is already used by repository %q", effectivePath, other),
			}
		}
		repoPaths[effectivePath] = repo.Name
	}

	// Validate projects
	projectNames := make(map[string]bool)
	for i, proj := range cfg.Projects {
		if err := validateProject(&proj, i, repoNames); err != nil {
			return err
		}
		if projectNames[proj.Name] {
			return &ValidationError{
				Field:   fmt.Sprintf("project[%d].name", i),
				Message: fmt.Sprintf("duplicate project name: %s", proj.Name),
			}
		}
		projectNames[proj.Name] = true
	}

	return nil
}

// validateGeneral range-checks the global settings. Zero values are allowed
// (they are either valid or replaced by defaults at load time); negative
// values are always configuration mistakes.
func validateGeneral(cfg *Config) error {
	if cfg.General.Timeout < 0 {
		return &ValidationError{Field: "general.timeout", Message: "timeout must not be negative"}
	}
	if cfg.HTTP.RetryAttempts < 0 {
		return &ValidationError{Field: "http.retry_attempts", Message: "retry_attempts must not be negative"}
	}
	if cfg.HTTP.RetryDelay < 0 {
		return &ValidationError{Field: "http.retry_delay", Message: "retry_delay must not be negative"}
	}
	if cfg.Git.CloneDepth < 0 {
		return &ValidationError{Field: "git.clone_depth", Message: "clone_depth must not be negative"}
	}
	return nil
}

func validateRepository(repo *Repository, index int) error {
	prefix := fmt.Sprintf("repository[%d]", index)

	if repo.Name == "" {
		return &ValidationError{Field: prefix + ".name", Message: "name is required"}
	}

	if repo.URL == "" {
		return &ValidationError{Field: prefix + ".url", Message: "url is required"}
	}

	if err := validateURL(repo.URL); err != nil {
		return &ValidationError{Field: prefix + ".url", Message: err.Error()}
	}

	if repo.Type == "" {
		return &ValidationError{Field: prefix + ".type", Message: "type is required"}
	}

	if repo.Type != RepoTypeGit && repo.Type != RepoTypeHTTP {
		return &ValidationError{
			Field:   prefix + ".type",
			Message: fmt.Sprintf("invalid type: %s (must be 'git' or 'http')", repo.Type),
		}
	}

	// The effective path (explicit path, or the repository name as fallback)
	// must stay inside work_dir.
	if err := validateRepoPath(repo.GetEffectivePath()); err != nil {
		return &ValidationError{Field: prefix + ".path", Message: err.Error()}
	}

	if repo.Depth != nil && *repo.Depth < 0 {
		return &ValidationError{Field: prefix + ".depth", Message: "depth must not be negative"}
	}

	// Check for conflicting ref specifications
	refCount := 0
	if repo.Branch != "" {
		refCount++
	}
	if repo.Tag != "" {
		refCount++
	}
	if repo.Commit != "" {
		refCount++
	}
	if refCount > 1 {
		return &ValidationError{
			Field:   prefix,
			Message: "only one of branch, tag, or commit can be specified",
		}
	}

	return nil
}

// validateRepoPath ensures a repository's local path is relative and stays
// within the workspace directory.
func validateRepoPath(path string) error {
	if filepath.IsAbs(path) {
		return fmt.Errorf("path must be relative to work_dir, got absolute path: %s", path)
	}
	clean := filepath.Clean(path)
	if clean == "." {
		return fmt.Errorf("path %q resolves to work_dir itself", path)
	}
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path %q escapes work_dir", path)
	}
	return nil
}

func validateProject(proj *Project, index int, repoNames map[string]bool) error {
	prefix := fmt.Sprintf("project[%d]", index)

	if proj.Name == "" {
		return &ValidationError{Field: prefix + ".name", Message: "name is required"}
	}

	// Allow empty projects - repositories can be added later
	// Validate that any listed repos exist
	for i, repoName := range proj.Repositories {
		if !repoNames[repoName] {
			return &ValidationError{
				Field:   fmt.Sprintf("%s.repositories[%d]", prefix, i),
				Message: fmt.Sprintf("unknown repository: %s", repoName),
			}
		}
	}

	return nil
}

func validateURL(rawURL string) error {
	// SCP-style SSH URLs ("user@host:path") have no scheme and cannot be
	// parsed by net/url. Any user is allowed, not just "git".
	if !strings.Contains(rawURL, "://") && strings.Contains(rawURL, "@") {
		return validateSCPStyleURL(rawURL)
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	if u.Scheme == "" {
		return fmt.Errorf("URL must have a scheme (http, https, git, ssh, or file) or be an SSH URL of the form user@host:path")
	}

	// file:// URLs don't require a host (local paths)
	if u.Scheme == "file" {
		if u.Path == "" {
			return fmt.Errorf("file:// URL must have a path")
		}
		return nil
	}

	if u.Host == "" {
		return fmt.Errorf("URL must have a host")
	}

	return nil
}

// validateSCPStyleURL validates SSH URLs of the form "user@host:path"
// (e.g. "git@github.com:org/repo.git" or "deploy@server.local:repo.git").
func validateSCPStyleURL(rawURL string) error {
	at := strings.Index(rawURL, "@")
	user, rest := rawURL[:at], rawURL[at+1:]
	if user == "" {
		return fmt.Errorf("invalid SSH URL %q: missing user before '@' (expected user@host:path)", rawURL)
	}
	colon := strings.Index(rest, ":")
	if colon < 0 {
		return fmt.Errorf("invalid SSH URL %q: missing ':path' after host (expected user@host:path)", rawURL)
	}
	host, path := rest[:colon], rest[colon+1:]
	if host == "" {
		return fmt.Errorf("invalid SSH URL %q: missing host (expected user@host:path)", rawURL)
	}
	if strings.ContainsAny(host, "/\\") {
		return fmt.Errorf("invalid SSH URL %q: host must not contain path separators", rawURL)
	}
	if path == "" {
		return fmt.Errorf("invalid SSH URL %q: missing repository path after ':' (expected user@host:path)", rawURL)
	}
	return nil
}
