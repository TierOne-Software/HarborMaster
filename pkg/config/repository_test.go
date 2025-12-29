package config

import "testing"

func TestRepositoryType_Constants(t *testing.T) {
	if RepoTypeGit != "git" {
		t.Errorf("expected RepoTypeGit 'git', got '%s'", RepoTypeGit)
	}
	if RepoTypeHTTP != "http" {
		t.Errorf("expected RepoTypeHTTP 'http', got '%s'", RepoTypeHTTP)
	}
}

func TestRepository_GetEffectiveRef(t *testing.T) {
	tests := []struct {
		name          string
		repo          Repository
		defaultBranch string
		expected      string
	}{
		{
			name:          "commit takes priority",
			repo:          Repository{Branch: "main", Tag: "v1.0", Commit: "abc123"},
			defaultBranch: "develop",
			expected:      "abc123",
		},
		{
			name:          "tag over branch",
			repo:          Repository{Branch: "main", Tag: "v1.0"},
			defaultBranch: "develop",
			expected:      "v1.0",
		},
		{
			name:          "branch specified",
			repo:          Repository{Branch: "feature"},
			defaultBranch: "develop",
			expected:      "feature",
		},
		{
			name:          "uses default branch",
			repo:          Repository{},
			defaultBranch: "develop",
			expected:      "develop",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.repo.GetEffectiveRef(tt.defaultBranch)
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestRepository_GetEffectivePath(t *testing.T) {
	tests := []struct {
		name     string
		repo     Repository
		expected string
	}{
		{
			name:     "path specified",
			repo:     Repository{Name: "my-repo", Path: "custom/path"},
			expected: "custom/path",
		},
		{
			name:     "uses name as default",
			repo:     Repository{Name: "my-repo"},
			expected: "my-repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.repo.GetEffectivePath()
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestRepository_IsShallow(t *testing.T) {
	trueBool := true
	falseBool := false

	tests := []struct {
		name           string
		repo           Repository
		defaultShallow bool
		expected       bool
	}{
		{
			name:           "override true",
			repo:           Repository{Shallow: &trueBool},
			defaultShallow: false,
			expected:       true,
		},
		{
			name:           "override false",
			repo:           Repository{Shallow: &falseBool},
			defaultShallow: true,
			expected:       false,
		},
		{
			name:           "use default true",
			repo:           Repository{},
			defaultShallow: true,
			expected:       true,
		},
		{
			name:           "use default false",
			repo:           Repository{},
			defaultShallow: false,
			expected:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.repo.IsShallow(tt.defaultShallow)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestRepository_GetDepth(t *testing.T) {
	depth5 := 5

	tests := []struct {
		name         string
		repo         Repository
		defaultDepth int
		expected     int
	}{
		{
			name:         "override depth",
			repo:         Repository{Depth: &depth5},
			defaultDepth: 1,
			expected:     5,
		},
		{
			name:         "use default",
			repo:         Repository{},
			defaultDepth: 1,
			expected:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.repo.GetDepth(tt.defaultDepth)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestRepository_HasSubmodules(t *testing.T) {
	trueBool := true
	falseBool := false

	tests := []struct {
		name              string
		repo              Repository
		defaultSubmodules bool
		expected          bool
	}{
		{
			name:              "override true",
			repo:              Repository{Submodules: &trueBool},
			defaultSubmodules: false,
			expected:          true,
		},
		{
			name:              "override false",
			repo:              Repository{Submodules: &falseBool},
			defaultSubmodules: true,
			expected:          false,
		},
		{
			name:              "use default",
			repo:              Repository{},
			defaultSubmodules: true,
			expected:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.repo.HasSubmodules(tt.defaultSubmodules)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
