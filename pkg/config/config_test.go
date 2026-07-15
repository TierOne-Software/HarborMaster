package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoad_ValidConfig(t *testing.T) {
	cfg, err := Load("testdata/valid.toml")
	if err != nil {
		t.Fatalf("failed to load valid config: %v", err)
	}

	// Check general settings
	if cfg.General.WorkDir != "/tmp/workspace" {
		t.Errorf("expected WorkDir '/tmp/workspace', got '%s'", cfg.General.WorkDir)
	}
	if cfg.General.CacheDir != "/tmp/cache" {
		t.Errorf("expected CacheDir '/tmp/cache', got '%s'", cfg.General.CacheDir)
	}
	if cfg.General.Timeout != 5*time.Minute {
		t.Errorf("expected Timeout 5m, got %v", cfg.General.Timeout)
	}
	if cfg.General.DefaultBranch != "develop" {
		t.Errorf("expected DefaultBranch 'develop', got '%s'", cfg.General.DefaultBranch)
	}
	if cfg.General.RecurseSubmodule != false {
		t.Error("expected RecurseSubmodule false")
	}

	// Check HTTP settings
	if cfg.HTTP.UserAgent != "TestAgent/1.0" {
		t.Errorf("expected UserAgent 'TestAgent/1.0', got '%s'", cfg.HTTP.UserAgent)
	}
	if cfg.HTTP.RetryAttempts != 5 {
		t.Errorf("expected RetryAttempts 5, got %d", cfg.HTTP.RetryAttempts)
	}
	if cfg.HTTP.RetryDelay != 3*time.Second {
		t.Errorf("expected RetryDelay 3s, got %v", cfg.HTTP.RetryDelay)
	}

	// Check Git settings
	if cfg.Git.ShallowClone != false {
		t.Error("expected ShallowClone false")
	}
	if cfg.Git.CloneDepth != 10 {
		t.Errorf("expected CloneDepth 10, got %d", cfg.Git.CloneDepth)
	}

	// Check repositories
	if len(cfg.Repositories) != 3 {
		t.Fatalf("expected 3 repositories, got %d", len(cfg.Repositories))
	}

	// Check projects
	if len(cfg.Projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(cfg.Projects))
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("testdata/nonexistent.toml")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLoad_Defaults(t *testing.T) {
	// Create a minimal config file
	content := `
[[repository]]
name = "test"
url = "https://github.com/test/test.git"
type = "git"
`
	tmpFile := filepath.Join(t.TempDir(), "minimal.toml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("failed to load minimal config: %v", err)
	}

	// Check defaults are applied
	if cfg.General.Timeout != DefaultTimeout {
		t.Errorf("expected default Timeout %v, got %v", DefaultTimeout, cfg.General.Timeout)
	}
	if cfg.General.DefaultBranch != DefaultBranch {
		t.Errorf("expected default branch '%s', got '%s'", DefaultBranch, cfg.General.DefaultBranch)
	}
	if cfg.Git.CloneDepth != DefaultCloneDepth {
		t.Errorf("expected default CloneDepth %d, got %d", DefaultCloneDepth, cfg.Git.CloneDepth)
	}
	if cfg.HTTP.RetryAttempts != DefaultRetryAttempts {
		t.Errorf("expected default RetryAttempts %d, got %d", DefaultRetryAttempts, cfg.HTTP.RetryAttempts)
	}
}

func TestConfig_GetRepository(t *testing.T) {
	cfg, err := Load("testdata/valid.toml")
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Found
	repo, ok := cfg.GetRepository("repo1")
	if !ok {
		t.Error("expected to find repo1")
	}
	if repo.Name != "repo1" {
		t.Errorf("expected name 'repo1', got '%s'", repo.Name)
	}

	// Not found
	_, ok = cfg.GetRepository("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent repo")
	}
}

func TestConfig_GetProject(t *testing.T) {
	cfg, err := Load("testdata/valid.toml")
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Found
	proj, ok := cfg.GetProject("core")
	if !ok {
		t.Error("expected to find core project")
	}
	if len(proj.Repositories) != 2 {
		t.Errorf("expected 2 repos in core project, got %d", len(proj.Repositories))
	}

	// Not found
	_, ok = cfg.GetProject("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent project")
	}
}

func TestConfig_AddRepository(t *testing.T) {
	cfg := NewDefaultConfig()

	repo := Repository{
		Name: "new-repo",
		URL:  "https://github.com/test/new.git",
		Type: RepoTypeGit,
	}

	err := cfg.AddRepository(repo)
	if err != nil {
		t.Fatalf("failed to add repository: %v", err)
	}

	// Verify it was added
	found, ok := cfg.GetRepository("new-repo")
	if !ok {
		t.Error("expected to find new-repo after adding")
	}
	if found.URL != repo.URL {
		t.Errorf("expected URL '%s', got '%s'", repo.URL, found.URL)
	}

	// Try to add duplicate
	err = cfg.AddRepository(repo)
	if err == nil {
		t.Error("expected error when adding duplicate repository")
	}
}

func TestConfig_RemoveRepository(t *testing.T) {
	cfg := NewDefaultConfig()
	cfg.Repositories = []Repository{
		{Name: "repo1", URL: "https://example.com/1.git", Type: RepoTypeGit},
		{Name: "repo2", URL: "https://example.com/2.git", Type: RepoTypeGit},
	}

	err := cfg.RemoveRepository("repo1")
	if err != nil {
		t.Fatalf("failed to remove repository: %v", err)
	}

	if len(cfg.Repositories) != 1 {
		t.Errorf("expected 1 repository, got %d", len(cfg.Repositories))
	}
	if cfg.Repositories[0].Name != "repo2" {
		t.Errorf("expected repo2 to remain, got '%s'", cfg.Repositories[0].Name)
	}

	// Try to remove nonexistent
	err = cfg.RemoveRepository("nonexistent")
	if err == nil {
		t.Error("expected error when removing nonexistent repository")
	}
}

func TestConfig_GetRepositoriesForProject(t *testing.T) {
	cfg, err := Load("testdata/valid.toml")
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	repos, err := cfg.GetRepositoriesForProject("core")
	if err != nil {
		t.Fatalf("failed to get repos for project: %v", err)
	}

	if len(repos) != 2 {
		t.Errorf("expected 2 repos, got %d", len(repos))
	}

	// Nonexistent project
	_, err = cfg.GetRepositoriesForProject("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent project")
	}
}

func TestConfig_GetRepositoriesByTag(t *testing.T) {
	cfg, err := Load("testdata/valid.toml")
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	repos := cfg.GetRepositoriesByTag("backend")
	if len(repos) != 1 {
		t.Errorf("expected 1 repo with 'backend' tag, got %d", len(repos))
	}
	if repos[0].Name != "repo1" {
		t.Errorf("expected repo1, got '%s'", repos[0].Name)
	}

	// Nonexistent tag
	repos = cfg.GetRepositoriesByTag("nonexistent")
	if len(repos) != 0 {
		t.Errorf("expected 0 repos for nonexistent tag, got %d", len(repos))
	}
}

func TestConfig_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".harbormaster.toml")

	// Create config
	cfg := NewDefaultConfig()
	cfg.Repositories = []Repository{
		{Name: "test-repo", URL: "https://github.com/test/repo.git", Type: RepoTypeGit, Branch: "main"},
	}
	cfg.Projects = []Project{
		{Name: "test-project", Repositories: []string{"test-repo"}},
	}

	// Save
	if err := cfg.SaveTo(configPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Load and verify
	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load saved config: %v", err)
	}

	if len(loaded.Repositories) != 1 {
		t.Errorf("expected 1 repository, got %d", len(loaded.Repositories))
	}
	if loaded.Repositories[0].Name != "test-repo" {
		t.Errorf("expected 'test-repo', got '%s'", loaded.Repositories[0].Name)
	}
}

func TestLoad_UnknownKeysRejected(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantKey string
	}{
		{
			name: "misspelled general key",
			content: `
[general]
default_brnach = "main"
`,
			wantKey: "default_brnach",
		},
		{
			name: "misspelled repository table",
			content: `
[[repositories]]
name = "test"
url = "https://github.com/test/test.git"
type = "git"
`,
			wantKey: "repositories",
		},
		{
			name: "unknown repository key",
			content: `
[[repository]]
name = "test"
url = "https://github.com/test/test.git"
type = "git"
brnch = "main"
`,
			wantKey: "brnch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(tmpFile, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("failed to write temp file: %v", err)
			}

			_, err := Load(tmpFile)
			if err == nil {
				t.Fatal("expected error for unknown keys")
			}
			if !strings.Contains(err.Error(), tt.wantKey) {
				t.Errorf("expected error to mention %q, got: %v", tt.wantKey, err)
			}
		})
	}
}

func TestSaveTo_PreservesExistingFileOnError(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ConfigFileName)

	cfg := NewDefaultConfig()
	if err := cfg.SaveTo(configPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}
	original, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}

	// Make the directory unwritable so the save cannot create its temp file.
	if os.Geteuid() == 0 {
		t.Skip("running as root; directory permissions are not enforced")
	}
	if err := os.Chmod(tmpDir, 0o555); err != nil {
		t.Fatalf("failed to chmod dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(tmpDir, 0o755) })

	if err := cfg.SaveTo(configPath); err == nil {
		t.Fatal("expected save to fail in read-only directory")
	}

	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to re-read config: %v", err)
	}
	if string(after) != string(original) {
		t.Error("existing config file was modified by a failed save")
	}
}

func TestSaveTo_FilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ConfigFileName)

	// New files are created 0600.
	cfg := NewDefaultConfig()
	if err := cfg.SaveTo(configPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("failed to stat config: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("expected new file mode 0600, got %o", perm)
	}

	// Existing file permissions are preserved.
	if err := os.Chmod(configPath, 0o644); err != nil {
		t.Fatalf("failed to chmod: %v", err)
	}
	if err := cfg.SaveTo(configPath); err != nil {
		t.Fatalf("failed to re-save config: %v", err)
	}
	info, err = os.Stat(configPath)
	if err != nil {
		t.Fatalf("failed to stat config: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Errorf("expected preserved file mode 0644, got %o", perm)
	}
}

func TestSaveTo_NoTempFileLeftover(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ConfigFileName)

	cfg := NewDefaultConfig()
	if err := cfg.SaveTo(configPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to read dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != ConfigFileName {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("expected only %s in dir, got %v", ConfigFileName, names)
	}
}

func TestSaveTo_PreservesRelativeWorkDir(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ConfigFileName)
	if err := os.MkdirAll(filepath.Join(tmpDir, "ws"), 0o755); err != nil {
		t.Fatal(err)
	}

	content := `
[general]
work_dir = "./ws"
cache_dir = "./cache"
`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if err := cfg.Save(); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `work_dir = "./ws"`) {
		t.Errorf("expected relative work_dir to be preserved, got:\n%s", data)
	}
	if !strings.Contains(string(data), `cache_dir = "./cache"`) {
		t.Errorf("expected relative cache_dir to be preserved, got:\n%s", data)
	}
}

func TestSaveTo_ProgrammaticDirChangeIsSaved(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ConfigFileName)
	if err := os.MkdirAll(filepath.Join(tmpDir, "ws"), 0o755); err != nil {
		t.Fatal(err)
	}

	content := `
[general]
work_dir = "./ws"
`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Change the runtime paths programmatically.
	newWorkDir := filepath.Join(tmpDir, "other-ws")
	newCacheDir := filepath.Join(tmpDir, "cache")
	cfg.General.WorkDir = newWorkDir
	cfg.General.CacheDir = newCacheDir

	if err := cfg.Save(); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	reloaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to reload config: %v", err)
	}
	if reloaded.General.WorkDir != newWorkDir {
		t.Errorf("expected WorkDir %q after round-trip, got %q", newWorkDir, reloaded.General.WorkDir)
	}
	if reloaded.General.CacheDir != newCacheDir {
		t.Errorf("expected CacheDir %q after round-trip, got %q", newCacheDir, reloaded.General.CacheDir)
	}
}

func TestLoad_RelativePathSurvivesChdir(t *testing.T) {
	tmpDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(tmpDir, ConfigFileName)

	cfg := NewDefaultConfig()
	if err := cfg.SaveTo(configPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	// Load via a relative path...
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(ConfigFileName)
	if err != nil {
		t.Fatalf("failed to load config via relative path: %v", err)
	}
	if !filepath.IsAbs(loaded.Path()) {
		t.Errorf("expected Path() to be absolute, got %q", loaded.Path())
	}

	// ...then chdir elsewhere and save: it must write to the original file,
	// not to a new file in the new working directory.
	otherDir := t.TempDir()
	if err := os.Chdir(otherDir); err != nil {
		t.Fatal(err)
	}
	loaded.General.DefaultBranch = "trunk"
	if err := loaded.Save(); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	if _, err := os.Stat(filepath.Join(otherDir, ConfigFileName)); !os.IsNotExist(err) {
		t.Error("save after chdir wrote to the new working directory")
	}
	reloaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to reload config: %v", err)
	}
	if reloaded.General.DefaultBranch != "trunk" {
		t.Errorf("expected saved change to land in original file, got branch %q", reloaded.General.DefaultBranch)
	}
}

func TestNewDefaultConfigE(t *testing.T) {
	cfg, err := NewDefaultConfigE()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.General.WorkDir != cwd {
		t.Errorf("expected WorkDir %q, got %q", cwd, cfg.General.WorkDir)
	}
}

func TestNewDefaultConfig(t *testing.T) {
	cfg := NewDefaultConfig()

	if cfg.General.Timeout != DefaultTimeout {
		t.Errorf("expected Timeout %v, got %v", DefaultTimeout, cfg.General.Timeout)
	}
	if cfg.General.DefaultBranch != DefaultBranch {
		t.Errorf("expected DefaultBranch '%s', got '%s'", DefaultBranch, cfg.General.DefaultBranch)
	}
	if cfg.Git.ShallowClone != true {
		t.Error("expected ShallowClone true by default")
	}
}

func TestConfig_AddProject(t *testing.T) {
	cfg := NewDefaultConfig()
	cfg.Repositories = []Repository{
		{Name: "repo1", URL: "https://example.com/1.git", Type: RepoTypeGit},
	}

	proj := Project{
		Name:         "test-project",
		Repositories: []string{"repo1"},
		Tags:         []string{"backend"},
	}

	err := cfg.AddProject(proj)
	if err != nil {
		t.Fatalf("failed to add project: %v", err)
	}

	// Verify it was added
	found, ok := cfg.GetProject("test-project")
	if !ok {
		t.Error("expected to find test-project after adding")
	}
	if len(found.Repositories) != 1 {
		t.Errorf("expected 1 repository, got %d", len(found.Repositories))
	}

	// Try to add duplicate
	err = cfg.AddProject(proj)
	if err == nil {
		t.Error("expected error when adding duplicate project")
	}
}

func TestConfig_RemoveProject(t *testing.T) {
	cfg := NewDefaultConfig()
	cfg.Projects = []Project{
		{Name: "proj1", Repositories: []string{}},
		{Name: "proj2", Repositories: []string{}},
	}

	err := cfg.RemoveProject("proj1")
	if err != nil {
		t.Fatalf("failed to remove project: %v", err)
	}

	if len(cfg.Projects) != 1 {
		t.Errorf("expected 1 project, got %d", len(cfg.Projects))
	}
	if cfg.Projects[0].Name != "proj2" {
		t.Errorf("expected proj2 to remain, got '%s'", cfg.Projects[0].Name)
	}

	// Try to remove nonexistent
	err = cfg.RemoveProject("nonexistent")
	if err == nil {
		t.Error("expected error when removing nonexistent project")
	}
}

func TestConfig_AddRepoToProject(t *testing.T) {
	cfg := NewDefaultConfig()
	cfg.Repositories = []Repository{
		{Name: "repo1", URL: "https://example.com/1.git", Type: RepoTypeGit},
		{Name: "repo2", URL: "https://example.com/2.git", Type: RepoTypeGit},
	}
	cfg.Projects = []Project{
		{Name: "proj1", Repositories: []string{"repo1"}},
	}

	// Add existing repo to project
	err := cfg.AddRepoToProject("proj1", "repo2")
	if err != nil {
		t.Fatalf("failed to add repo to project: %v", err)
	}

	proj, _ := cfg.GetProject("proj1")
	if len(proj.Repositories) != 2 {
		t.Errorf("expected 2 repositories, got %d", len(proj.Repositories))
	}

	// Try to add duplicate
	err = cfg.AddRepoToProject("proj1", "repo2")
	if err == nil {
		t.Error("expected error when adding duplicate repo to project")
	}

	// Try to add nonexistent repo
	err = cfg.AddRepoToProject("proj1", "nonexistent")
	if err == nil {
		t.Error("expected error when adding nonexistent repo")
	}

	// Try to add to nonexistent project
	err = cfg.AddRepoToProject("nonexistent", "repo1")
	if err == nil {
		t.Error("expected error when adding to nonexistent project")
	}
}

func TestConfig_RemoveRepoFromProject(t *testing.T) {
	cfg := NewDefaultConfig()
	cfg.Projects = []Project{
		{Name: "proj1", Repositories: []string{"repo1", "repo2"}},
	}

	err := cfg.RemoveRepoFromProject("proj1", "repo1")
	if err != nil {
		t.Fatalf("failed to remove repo from project: %v", err)
	}

	proj, _ := cfg.GetProject("proj1")
	if len(proj.Repositories) != 1 {
		t.Errorf("expected 1 repository, got %d", len(proj.Repositories))
	}
	if proj.Repositories[0] != "repo2" {
		t.Errorf("expected repo2 to remain, got '%s'", proj.Repositories[0])
	}

	// Try to remove nonexistent repo from project
	err = cfg.RemoveRepoFromProject("proj1", "nonexistent")
	if err == nil {
		t.Error("expected error when removing nonexistent repo from project")
	}

	// Try to remove from nonexistent project
	err = cfg.RemoveRepoFromProject("nonexistent", "repo2")
	if err == nil {
		t.Error("expected error when removing from nonexistent project")
	}
}
