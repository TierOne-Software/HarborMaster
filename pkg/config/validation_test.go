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

func TestValidateConfig_AbsoluteRepoPath(t *testing.T) {
	cfg := &Config{
		Repositories: []Repository{
			{Name: "repo1", URL: "https://github.com/test/repo.git", Type: RepoTypeGit, Path: "/etc/cron.d/repo1"},
		},
	}

	err := ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected error for absolute repository path")
	}
	if !strings.Contains(err.Error(), "absolute") {
		t.Errorf("expected absolute-path error, got: %v", err)
	}
}

func TestValidateConfig_RepoPathEscapesWorkDir(t *testing.T) {
	escaping := []string{
		"..",
		"../evil",
		"sub/../../evil",
		"a/b/../../../evil",
	}
	for _, p := range escaping {
		t.Run(p, func(t *testing.T) {
			cfg := &Config{
				Repositories: []Repository{
					{Name: "repo1", URL: "https://github.com/test/repo.git", Type: RepoTypeGit, Path: p},
				},
			}
			if err := ValidateConfig(cfg); err == nil {
				t.Fatalf("expected error for escaping path %q", p)
			}
		})
	}

	// Paths with internal .. that stay inside work_dir are fine.
	cfg := &Config{
		Repositories: []Repository{
			{Name: "repo1", URL: "https://github.com/test/repo.git", Type: RepoTypeGit, Path: "a/../b/repo1"},
		},
	}
	if err := ValidateConfig(cfg); err != nil {
		t.Errorf("expected internal .. that stays inside work_dir to be valid, got: %v", err)
	}
}

func TestValidateConfig_RepoPathIsWorkDir(t *testing.T) {
	cfg := &Config{
		Repositories: []Repository{
			{Name: "repo1", URL: "https://github.com/test/repo.git", Type: RepoTypeGit, Path: "./"},
		},
	}
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected error for path resolving to work_dir itself")
	}
}

func TestValidateConfig_RepoNameFallbackPathValidated(t *testing.T) {
	// With no explicit path, the name is used as the path and must obey the
	// same rules.
	cfg := &Config{
		Repositories: []Repository{
			{Name: "../evil", URL: "https://github.com/test/repo.git", Type: RepoTypeGit},
		},
	}
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected error when repository name escapes work_dir")
	}
}

func TestValidateConfig_DuplicateEffectivePaths(t *testing.T) {
	// Two repos with the same explicit path.
	cfg := &Config{
		Repositories: []Repository{
			{Name: "repo1", URL: "https://github.com/test/repo1.git", Type: RepoTypeGit, Path: "shared/dir"},
			{Name: "repo2", URL: "https://github.com/test/repo2.git", Type: RepoTypeGit, Path: "shared/dir"},
		},
	}
	err := ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected error for duplicate repository paths")
	}
	if !strings.Contains(err.Error(), "already used") {
		t.Errorf("expected duplicate-path error, got: %v", err)
	}

	// One repo's explicit path collides with another repo's name fallback.
	cfg = &Config{
		Repositories: []Repository{
			{Name: "repo1", URL: "https://github.com/test/repo1.git", Type: RepoTypeGit},
			{Name: "repo2", URL: "https://github.com/test/repo2.git", Type: RepoTypeGit, Path: "./repo1"},
		},
	}
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected error for explicit path colliding with name fallback")
	}
}

func TestValidateURL_SCPStyle(t *testing.T) {
	valid := []string{
		"git@github.com:org/repo.git",
		"deploy@server.local:repo.git",
		"jenkins-ci@git.internal.example.com:team/project.git",
	}
	for _, u := range valid {
		t.Run("valid/"+u, func(t *testing.T) {
			if err := validateURL(u); err != nil {
				t.Errorf("expected %q to be valid: %v", u, err)
			}
		})
	}

	invalid := []string{
		"git@",                     // nothing after user
		"git@github.com",           // missing :path
		"git@github.com:",          // empty path
		"git@:org/repo.git",        // empty host
		"@github.com:org/repo.git", // empty user
		"git@host/with:path",       // separator in host
	}
	for _, u := range invalid {
		t.Run("invalid/"+u, func(t *testing.T) {
			if err := validateURL(u); err == nil {
				t.Errorf("expected %q to be rejected", u)
			}
		})
	}
}

func TestValidateConfig_NegativeRanges(t *testing.T) {
	base := func() *Config {
		return &Config{
			Repositories: []Repository{
				{Name: "repo1", URL: "https://github.com/test/repo.git", Type: RepoTypeGit},
			},
		}
	}

	t.Run("negative timeout", func(t *testing.T) {
		cfg := base()
		cfg.General.Timeout = -1
		if err := ValidateConfig(cfg); err == nil {
			t.Error("expected error for negative timeout")
		}
	})

	t.Run("negative retry_attempts", func(t *testing.T) {
		cfg := base()
		cfg.HTTP.RetryAttempts = -1
		if err := ValidateConfig(cfg); err == nil {
			t.Error("expected error for negative retry_attempts")
		}
	})

	t.Run("negative retry_delay", func(t *testing.T) {
		cfg := base()
		cfg.HTTP.RetryDelay = -1
		if err := ValidateConfig(cfg); err == nil {
			t.Error("expected error for negative retry_delay")
		}
	})

	t.Run("negative clone_depth", func(t *testing.T) {
		cfg := base()
		cfg.Git.CloneDepth = -1
		if err := ValidateConfig(cfg); err == nil {
			t.Error("expected error for negative clone_depth")
		}
	})

	t.Run("negative repo depth", func(t *testing.T) {
		cfg := base()
		depth := -1
		cfg.Repositories[0].Depth = &depth
		if err := ValidateConfig(cfg); err == nil {
			t.Error("expected error for negative repository depth")
		}
	})

	t.Run("zero values allowed", func(t *testing.T) {
		cfg := base()
		if err := ValidateConfig(cfg); err != nil {
			t.Errorf("zero-valued settings should be valid: %v", err)
		}
	})
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
