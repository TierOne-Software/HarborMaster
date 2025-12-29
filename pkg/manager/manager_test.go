package manager

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/tierone/harbormaster/pkg/config"
	"github.com/tierone/harbormaster/pkg/lockfile"
)

// setupTestGitRepo creates a temporary git repository for testing
func setupTestGitRepo(t *testing.T, name string) string {
	t.Helper()

	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, name)

	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}

	commands := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test User"},
	}

	for _, cmd := range commands {
		c := exec.Command(cmd[0], cmd[1:]...)
		c.Dir = repoDir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("failed to run %v: %v\n%s", cmd, err, out)
		}
	}

	// Create initial commit
	testFile := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(testFile, []byte("# "+name), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	commitCmds := [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "Initial commit"},
	}

	for _, cmd := range commitCmds {
		c := exec.Command(cmd[0], cmd[1:]...)
		c.Dir = repoDir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("failed to run %v: %v\n%s", cmd, err, out)
		}
	}

	return repoDir
}

func TestNewRepositoryManager(t *testing.T) {
	cfg := config.NewDefaultConfig()
	mgr := NewRepositoryManager(cfg)

	if mgr.config != cfg {
		t.Error("expected config to be set")
	}
	if mgr.concurrent != 4 {
		t.Errorf("expected concurrent 4, got %d", mgr.concurrent)
	}
	if !mgr.interactive {
		t.Error("expected interactive to be true by default")
	}
}

func TestNewRepositoryManager_WithOptions(t *testing.T) {
	cfg := config.NewDefaultConfig()
	lf := lockfile.New()

	mgr := NewRepositoryManager(cfg,
		WithLockFile(lf),
		WithConcurrency(8),
		WithLocked(true),
		WithInteractive(false),
	)

	if mgr.lockFile != lf {
		t.Error("expected lock file to be set")
	}
	if mgr.concurrent != 8 {
		t.Errorf("expected concurrent 8, got %d", mgr.concurrent)
	}
	if !mgr.locked {
		t.Error("expected locked to be true")
	}
	if mgr.interactive {
		t.Error("expected interactive to be false")
	}
}

func TestRepositoryManager_Add(t *testing.T) {
	cfg := config.NewDefaultConfig()
	mgr := NewRepositoryManager(cfg)

	repo := config.Repository{
		Name: "test-repo",
		URL:  "https://github.com/test/repo.git",
		Type: config.RepoTypeGit,
	}

	err := mgr.Add(repo)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Verify it was added
	found, ok := cfg.GetRepository("test-repo")
	if !ok {
		t.Fatal("repository not found after Add")
	}
	if found.Path != "test-repo" {
		t.Errorf("expected path 'test-repo', got '%s'", found.Path)
	}
}

func TestRepositoryManager_Add_Validation(t *testing.T) {
	cfg := config.NewDefaultConfig()
	mgr := NewRepositoryManager(cfg)

	// Missing name
	err := mgr.Add(config.Repository{URL: "https://example.com"})
	if err == nil {
		t.Error("expected error for missing name")
	}

	// Missing URL
	err = mgr.Add(config.Repository{Name: "test"})
	if err == nil {
		t.Error("expected error for missing URL")
	}
}

func TestRepositoryManager_Remove(t *testing.T) {
	cfg := config.NewDefaultConfig()
	cfg.Repositories = []config.Repository{
		{Name: "repo1", URL: "https://example.com/1.git", Type: config.RepoTypeGit},
		{Name: "repo2", URL: "https://example.com/2.git", Type: config.RepoTypeGit},
	}

	lf := lockfile.New()
	lf.Update("repo1", lockfile.LockEntry{ResolvedSHA: "abc123"})
	lf.Update("repo2", lockfile.LockEntry{ResolvedSHA: "def456"})

	mgr := NewRepositoryManager(cfg, WithLockFile(lf))

	err := mgr.Remove("repo1")
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// Verify removed from config
	if len(cfg.Repositories) != 1 {
		t.Errorf("expected 1 repository, got %d", len(cfg.Repositories))
	}

	// Verify removed from lock file
	if lf.Has("repo1") {
		t.Error("expected repo1 to be removed from lock file")
	}
	if !lf.Has("repo2") {
		t.Error("expected repo2 to remain in lock file")
	}
}

func TestRepositoryManager_Status(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Create a test repo
	repoDir := setupTestGitRepo(t, "source-repo")
	workDir := t.TempDir()

	cfg := &config.Config{
		General: config.GeneralConfig{
			WorkDir:       workDir,
			DefaultBranch: "main",
		},
		Repositories: []config.Repository{
			{
				Name: "existing",
				URL:  repoDir,
				Type: config.RepoTypeGit,
				Path: "existing",
			},
			{
				Name: "missing",
				URL:  "https://github.com/test/missing.git",
				Type: config.RepoTypeGit,
				Path: "missing",
			},
		},
	}

	// Clone the existing repo
	cmd := exec.Command("git", "clone", repoDir, filepath.Join(workDir, "existing"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to clone: %v\n%s", err, out)
	}

	mgr := NewRepositoryManager(cfg)
	statuses, err := mgr.Status(Filter{All: true})
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}

	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(statuses))
	}

	// Find existing and missing
	var existingStatus, missingStatus *RepoStatus
	for i := range statuses {
		switch statuses[i].Name {
		case "existing":
			existingStatus = &statuses[i]
		case "missing":
			missingStatus = &statuses[i]
		}
	}

	if existingStatus == nil {
		t.Fatal("existing status not found")
	}
	if !existingStatus.Exists {
		t.Error("expected existing to exist")
	}
	if existingStatus.CurrentSHA == "" {
		t.Error("expected existing to have SHA")
	}

	if missingStatus == nil {
		t.Fatal("missing status not found")
	}
	if missingStatus.Exists {
		t.Error("expected missing to not exist")
	}
	if !missingStatus.NeedsUpdate {
		t.Error("expected missing to need update")
	}
}

func TestRepositoryManager_GetRepositories_Filter(t *testing.T) {
	cfg := &config.Config{
		Repositories: []config.Repository{
			{Name: "repo1", URL: "https://example.com/1.git", Type: config.RepoTypeGit, Tags: []string{"backend"}},
			{Name: "repo2", URL: "https://example.com/2.git", Type: config.RepoTypeGit, Tags: []string{"frontend"}},
			{Name: "repo3", URL: "https://example.com/3.git", Type: config.RepoTypeGit, Tags: []string{"backend", "core"}},
		},
		Projects: []config.Project{
			{Name: "proj1", Repositories: []string{"repo1", "repo2"}},
		},
	}

	mgr := NewRepositoryManager(cfg)

	// Test All filter
	repos, err := mgr.getRepositories(Filter{All: true})
	if err != nil {
		t.Fatalf("getRepositories failed: %v", err)
	}
	if len(repos) != 3 {
		t.Errorf("expected 3 repos with All, got %d", len(repos))
	}

	// Test Names filter
	repos, err = mgr.getRepositories(Filter{Names: []string{"repo1", "repo3"}})
	if err != nil {
		t.Fatalf("getRepositories failed: %v", err)
	}
	if len(repos) != 2 {
		t.Errorf("expected 2 repos with Names, got %d", len(repos))
	}

	// Test Projects filter
	repos, err = mgr.getRepositories(Filter{Projects: []string{"proj1"}})
	if err != nil {
		t.Fatalf("getRepositories failed: %v", err)
	}
	if len(repos) != 2 {
		t.Errorf("expected 2 repos with Projects, got %d", len(repos))
	}

	// Test Tags filter
	repos, err = mgr.getRepositories(Filter{Tags: []string{"backend"}})
	if err != nil {
		t.Fatalf("getRepositories failed: %v", err)
	}
	if len(repos) != 2 {
		t.Errorf("expected 2 repos with Tags, got %d", len(repos))
	}

	// Test nonexistent name
	_, err = mgr.getRepositories(Filter{Names: []string{"nonexistent"}})
	if err == nil {
		t.Error("expected error for nonexistent repo name")
	}
}

func TestRepositoryManager_Sync_SingleRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Create a test repo
	repoDir := setupTestGitRepo(t, "source-repo")
	workDir := t.TempDir()

	cfg := &config.Config{
		General: config.GeneralConfig{
			WorkDir:       workDir,
			DefaultBranch: "main",
			Timeout:       config.DefaultTimeout,
		},
		Git: config.GitConfig{
			ShallowClone: false,
			CloneDepth:   0,
		},
		Repositories: []config.Repository{
			{
				Name: "test-repo",
				URL:  repoDir,
				Type: config.RepoTypeGit,
				Path: "test-repo",
			},
		},
	}

	lf := lockfile.New()
	mgr := NewRepositoryManager(cfg,
		WithLockFile(lf),
		WithInteractive(false),
	)

	result, err := mgr.SyncOne("test-repo")
	if err != nil {
		t.Fatalf("SyncOne failed: %v", err)
	}

	if !result.Success {
		t.Fatalf("sync failed: %v", result.Error)
	}

	if result.CommitSHA == "" {
		t.Error("expected CommitSHA to be set")
	}

	// Verify cloned
	if _, err := os.Stat(filepath.Join(workDir, "test-repo", ".git")); err != nil {
		t.Error("expected .git directory")
	}

	// Verify lock file updated
	if !lf.Has("test-repo") {
		t.Error("expected lock file to have entry")
	}
}

func TestFilter(t *testing.T) {
	f := Filter{}

	// Empty filter
	if f.All || len(f.Names) > 0 || len(f.Projects) > 0 || len(f.Tags) > 0 {
		t.Error("expected empty filter")
	}

	// With values
	f = Filter{
		Names:    []string{"repo1"},
		Projects: []string{"proj1"},
		Tags:     []string{"tag1"},
		All:      true,
	}

	if !f.All {
		t.Error("expected All to be true")
	}
}

func TestRepoStatus(t *testing.T) {
	status := RepoStatus{
		Name:         "test",
		Path:         "/path/to/test",
		Exists:       true,
		CurrentSHA:   "abc123",
		LockedSHA:    "def456",
		RequestedRef: "main",
		Branch:       "main",
		IsDirty:      false,
		NeedsUpdate:  true,
		Error:        nil,
	}

	if status.Name != "test" {
		t.Errorf("expected Name 'test', got '%s'", status.Name)
	}
	if !status.NeedsUpdate {
		t.Error("expected NeedsUpdate to be true")
	}
}
