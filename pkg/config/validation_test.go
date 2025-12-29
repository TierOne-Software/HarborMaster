package config

import (
	"strings"
	"testing"
)

func TestValidateConfig_Valid(t *testing.T) {
	cfg := &Config{
		Repositories: []Repository{
			{Name: "repo1", URL: "https://github.com/test/repo.git", Type: RepoTypeGit},
			{Name: "repo2", URL: "https://example.com/file.tar.gz", Type: RepoTypeHTTP},
		},
		Projects: []Project{
			{Name: "proj1", Repositories: []string{"repo1"}},
		},
	}

	err := ValidateConfig(cfg)
	if err != nil {
		t.Errorf("expected valid config, got error: %v", err)
	}
}

func TestValidateConfig_DuplicateRepoName(t *testing.T) {
	cfg := &Config{
		Repositories: []Repository{
			{Name: "repo1", URL: "https://github.com/test/repo1.git", Type: RepoTypeGit},
			{Name: "repo1", URL: "https://github.com/test/repo2.git", Type: RepoTypeGit},
		},
	}

	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for duplicate repo name")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("expected duplicate error, got: %v", err)
	}
}

func TestValidateConfig_DuplicateProjectName(t *testing.T) {
	cfg := &Config{
		Repositories: []Repository{
			{Name: "repo1", URL: "https://github.com/test/repo.git", Type: RepoTypeGit},
		},
		Projects: []Project{
			{Name: "proj1", Repositories: []string{"repo1"}},
			{Name: "proj1", Repositories: []string{"repo1"}},
		},
	}

	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for duplicate project name")
	}
}

func TestValidateConfig_MissingRepoName(t *testing.T) {
	cfg := &Config{
		Repositories: []Repository{
			{Name: "", URL: "https://github.com/test/repo.git", Type: RepoTypeGit},
		},
	}

	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for missing repo name")
	}
}

func TestValidateConfig_MissingRepoURL(t *testing.T) {
	cfg := &Config{
		Repositories: []Repository{
			{Name: "repo1", URL: "", Type: RepoTypeGit},
		},
	}

	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for missing repo URL")
	}
}

func TestValidateConfig_InvalidRepoType(t *testing.T) {
	cfg := &Config{
		Repositories: []Repository{
			{Name: "repo1", URL: "https://github.com/test/repo.git", Type: "invalid"},
		},
	}

	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for invalid repo type")
	}
}

func TestValidateConfig_MissingRepoType(t *testing.T) {
	cfg := &Config{
		Repositories: []Repository{
			{Name: "repo1", URL: "https://github.com/test/repo.git", Type: ""},
		},
	}

	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for missing repo type")
	}
}

func TestValidateConfig_ConflictingRefs(t *testing.T) {
	cfg := &Config{
		Repositories: []Repository{
			{
				Name:   "repo1",
				URL:    "https://github.com/test/repo.git",
				Type:   RepoTypeGit,
				Branch: "main",
				Tag:    "v1.0",
			},
		},
	}

	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for conflicting refs (branch and tag)")
	}
}

func TestValidateConfig_InvalidURL(t *testing.T) {
	cfg := &Config{
		Repositories: []Repository{
			{Name: "repo1", URL: "not-a-valid-url", Type: RepoTypeGit},
		},
	}

	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestValidateConfig_GitSSHURL(t *testing.T) {
	cfg := &Config{
		Repositories: []Repository{
			{Name: "repo1", URL: "git@github.com:test/repo.git", Type: RepoTypeGit},
		},
	}

	err := ValidateConfig(cfg)
	if err != nil {
		t.Errorf("git@ SSH URLs should be valid: %v", err)
	}
}

func TestValidateConfig_ProjectUnknownRepo(t *testing.T) {
	cfg := &Config{
		Repositories: []Repository{
			{Name: "repo1", URL: "https://github.com/test/repo.git", Type: RepoTypeGit},
		},
		Projects: []Project{
			{Name: "proj1", Repositories: []string{"repo1", "nonexistent"}},
		},
	}

	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for unknown repo in project")
	}
}

func TestValidateConfig_EmptyProject(t *testing.T) {
	cfg := &Config{
		Repositories: []Repository{
			{Name: "repo1", URL: "https://github.com/test/repo.git", Type: RepoTypeGit},
		},
		Projects: []Project{
			{Name: "proj1", Repositories: []string{}},
		},
	}

	// Empty projects are now allowed (repos can be added later)
	err := ValidateConfig(cfg)
	if err != nil {
		t.Errorf("empty projects should be allowed: %v", err)
	}
}

func TestValidationError_Error(t *testing.T) {
	err := &ValidationError{
		Field:   "repository[0].name",
		Message: "name is required",
	}

	expected := "repository[0].name: name is required"
	if err.Error() != expected {
		t.Errorf("expected '%s', got '%s'", expected, err.Error())
	}
}
