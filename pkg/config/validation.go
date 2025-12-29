package config

import (
	"fmt"
	"net/url"
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
	// Validate repositories
	repoNames := make(map[string]bool)
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
	// Handle git@ SSH URLs
	if strings.HasPrefix(rawURL, "git@") {
		return nil
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	if u.Scheme == "" {
		return fmt.Errorf("URL must have a scheme (http, https, git, or file)")
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
