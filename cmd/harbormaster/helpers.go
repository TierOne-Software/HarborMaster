package main

import (
	"fmt"

	"github.com/tierone/harbormaster/pkg/config"
	"github.com/tierone/harbormaster/pkg/manager"
)

// buildFilter builds a repository filter from positional names and the
// --project/--tag flags. All provided selectors are combined as a union;
// if none are provided, all repositories match.
func buildFilter(names []string, project, tag string) manager.Filter {
	filter := manager.Filter{}
	if len(names) > 0 {
		filter.Names = names
	}
	if project != "" {
		filter.Projects = []string{project}
	}
	if tag != "" {
		filter.Tags = []string{tag}
	}
	if len(filter.Names) == 0 && len(filter.Projects) == 0 && len(filter.Tags) == 0 {
		filter.All = true
	}
	return filter
}

// shortSHA returns the first 8 characters of a SHA, or the SHA unchanged
// if it is shorter than that.
func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

// saveConfigValidated validates the in-memory configuration and persists it.
// Validation runs first so that a bad mutation can never be written to disk,
// which would make every subsequent command fail to load the config.
func saveConfigValidated() error {
	if err := config.ValidateConfig(cfg); err != nil {
		return fmt.Errorf("refusing to save invalid configuration: %w", err)
	}
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	return nil
}
