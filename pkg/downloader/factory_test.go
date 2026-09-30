package downloader

import (
	"testing"

	"github.com/tierone/harbormaster/pkg/config"
)

func TestRepoNameFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"git@github.com:HPControls/f5-viu-os-base.git", "f5-viu-os-base"},
		{"git@github.com:org/repo.git", "repo"},
		{"git@host:repo.git", "repo"}, // SCP-style, no slash after host
		{"https://github.com/org/repo.git", "repo"},
		{"https://github.com/org/repo", "repo"},
		{"https://github.com/org/repo/", "repo"},
		{"ssh://git@gitlab.com/group/sub/repo.git", "repo"},
		{"file:///home/mj/src/repo.git", "repo"},
		{"/home/mj/src/repo", "repo"},
		{"https://example.com/releases/download/v1/tool.tar.gz", "tool.tar.gz"},
		{"https://example.com/repo.git?token=abc", "repo"},
		{"", ""},
		{"/", ""},
	}
	for _, tt := range tests {
		if got := RepoNameFromURL(tt.url); got != tt.want {
			t.Errorf("RepoNameFromURL(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		name         string
		repoType     config.RepositoryType
		shouldError  bool
		expectedType string
	}{
		{"git downloader", config.RepoTypeGit, false, "git"},
		{"http downloader", config.RepoTypeHTTP, false, "http"},
		{"unknown type", config.RepositoryType("unknown"), true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dl, err := New(tt.repoType, DefaultOptions())
			if tt.shouldError {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if dl.Type() != tt.expectedType {
				t.Errorf("expected type '%s', got '%s'", tt.expectedType, dl.Type())
			}
		})
	}
}

func TestNewFromRepository(t *testing.T) {
	cfg := &config.Config{
		General: config.GeneralConfig{
			Timeout:          config.DefaultTimeout,
			RecurseSubmodule: true,
		},
		HTTP: config.HTTPConfig{
			UserAgent:     "TestAgent/1.0",
			RetryAttempts: 3,
			RetryDelay:    config.DefaultRetryDelay,
		},
		Git: config.GitConfig{
			ShallowClone: true,
			CloneDepth:   1,
		},
	}

	tests := []struct {
		name         string
		repo         *config.Repository
		expectedType string
	}{
		{
			name: "git repository",
			repo: &config.Repository{
				Name:   "test-git",
				URL:    "https://github.com/test/repo.git",
				Type:   config.RepoTypeGit,
				Branch: "main",
			},
			expectedType: "git",
		},
		{
			name: "http repository",
			repo: &config.Repository{
				Name: "test-http",
				URL:  "https://example.com/file.tar.gz",
				Type: config.RepoTypeHTTP,
			},
			expectedType: "http",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dl, err := NewFromRepository(tt.repo, cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if dl.Type() != tt.expectedType {
				t.Errorf("expected type '%s', got '%s'", tt.expectedType, dl.Type())
			}
		})
	}
}

func TestNewFromRepository_SeedsSourceURL(t *testing.T) {
	cfg := &config.Config{
		General: config.GeneralConfig{Timeout: config.DefaultTimeout},
	}

	repo := &config.Repository{
		Name: "test-http",
		URL:  "https://example.com/file.tar.gz",
		Type: config.RepoTypeHTTP,
	}

	dl, err := NewFromRepository(repo, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// A fresh downloader must know its source so Update works without a
	// prior Download call.
	httpDL, ok := dl.(*HTTPDownloader)
	if !ok {
		t.Fatalf("expected *HTTPDownloader, got %T", dl)
	}
	if httpDL.source != repo.URL {
		t.Errorf("expected source %q, got %q", repo.URL, httpDL.source)
	}
}

func TestDetectType(t *testing.T) {
	tests := []struct {
		url      string
		expected config.RepositoryType
	}{
		// Git URLs
		{"git@github.com:user/repo.git", config.RepoTypeGit},
		{"git://github.com/user/repo.git", config.RepoTypeGit},
		{"https://github.com/user/repo.git", config.RepoTypeGit},
		{"https://github.com/user/repo", config.RepoTypeGit},
		{"https://gitlab.com/user/repo.git", config.RepoTypeGit},
		{"https://bitbucket.org/user/repo.git", config.RepoTypeGit},

		{"ssh://git@github.com/user/repo", config.RepoTypeGit},

		// HTTP URLs (non-git hosts)
		{"https://example.com/file.tar.gz", config.RepoTypeHTTP},
		{"http://example.com/config.json", config.RepoTypeHTTP},

		// Release assets on git forges are plain file downloads.
		{"https://github.com/user/repo/releases/download/v1.0.0/tool.tar.gz", config.RepoTypeHTTP},
		{"https://github.com/user/repo/releases/download/v1.0.0/tool.tar.gz?token=abc", config.RepoTypeHTTP},

		// Query strings must not confuse the .git suffix check.
		{"https://example.com/user/repo.git?ref=main", config.RepoTypeGit},
		{"https://example.com/file.tar.gz?sig=xyz.git", config.RepoTypeHTTP},
		{"https://example.com/user/repo.git#fragment", config.RepoTypeGit},

		// Default to git for ambiguous
		{"some-path", config.RepoTypeGit},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			result := DetectType(tt.url)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()

	if opts.Depth != 1 {
		t.Errorf("expected Depth 1, got %d", opts.Depth)
	}
	if !opts.Shallow {
		t.Error("expected Shallow to be true")
	}
	if !opts.Submodules {
		t.Error("expected Submodules to be true")
	}
	if opts.UserAgent != "Harbormaster/1.0" {
		t.Errorf("expected UserAgent 'Harbormaster/1.0', got '%s'", opts.UserAgent)
	}
	if opts.RetryAttempts != 3 {
		t.Errorf("expected RetryAttempts 3, got %d", opts.RetryAttempts)
	}
}

func TestOptions_GetEffectiveRef(t *testing.T) {
	tests := []struct {
		name     string
		opts     Options
		expected string
	}{
		{
			name:     "commit takes priority",
			opts:     Options{Branch: "main", Tag: "v1.0", Commit: "abc123"},
			expected: "abc123",
		},
		{
			name:     "tag over branch",
			opts:     Options{Branch: "main", Tag: "v1.0"},
			expected: "v1.0",
		},
		{
			name:     "branch",
			opts:     Options{Branch: "develop"},
			expected: "develop",
		},
		{
			name:     "empty",
			opts:     Options{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.opts.GetEffectiveRef()
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}
