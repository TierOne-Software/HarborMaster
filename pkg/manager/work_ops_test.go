package manager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tierone/harbormaster/pkg/config"
	"github.com/tierone/harbormaster/pkg/work"
)

// setupWorkWorkspace creates a workdir containing n git repositories named
// repo1..repoN and returns the config and manager for them.
func setupWorkWorkspace(t *testing.T, n int) (*config.Config, *RepositoryManager) {
	t.Helper()
	requireGit(t)

	workDir := t.TempDir()
	repos := make([]config.Repository, 0, n)

	for i := 1; i <= n; i++ {
		name := "repo" + string(rune('0'+i))
		repoDir := filepath.Join(workDir, name)
		if err := os.MkdirAll(repoDir, 0755); err != nil {
			t.Fatalf("failed to create repo dir: %v", err)
		}
		gitCmd(t, repoDir, "init")
		gitCmd(t, repoDir, "config", "user.email", "test@test.com")
		gitCmd(t, repoDir, "config", "user.name", "Test User")
		addCommit(t, repoDir, "README.md", "# "+name)

		repos = append(repos, config.Repository{
			Name: name,
			URL:  "https://example.com/" + name + ".git",
			Type: config.RepoTypeGit,
			Path: name,
		})
	}

	cfg := &config.Config{
		General: config.GeneralConfig{
			WorkDir:       workDir,
			DefaultBranch: "main",
			Timeout:       config.DefaultTimeout,
		},
		Repositories: repos,
	}

	return cfg, NewRepositoryManager(cfg, WithInteractive(false))
}

func currentBranch(t *testing.T, dir string) string {
	t.Helper()
	return gitCmd(t, dir, "rev-parse", "--abbrev-ref", "HEAD")
}

func makeDirty(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("dirty change"), 0644); err != nil {
		t.Fatalf("failed to dirty repo: %v", err)
	}
}

func cleanRepo(t *testing.T, dir string) {
	t.Helper()
	gitCmd(t, dir, "checkout", "--", ".")
}

func TestWorkStart_HappyPath(t *testing.T) {
	cfg, mgr := setupWorkWorkspace(t, 2)
	repo1 := filepath.Join(cfg.General.WorkDir, "repo1")
	repo2 := filepath.Join(cfg.General.WorkDir, "repo2")

	orig1 := currentBranch(t, repo1)

	ws, err := mgr.WorkStart("feat", "feat", Filter{All: true})
	if err != nil {
		t.Fatalf("WorkStart failed: %v", err)
	}

	if len(ws.Repos) != 2 {
		t.Fatalf("expected 2 repos in session, got %d", len(ws.Repos))
	}

	for _, dir := range []string{repo1, repo2} {
		if got := currentBranch(t, dir); got != "feat" {
			t.Errorf("expected %s on branch feat, got %s", dir, got)
		}
	}

	wr, ok := ws.GetRepo("repo1")
	if !ok {
		t.Fatal("repo1 missing from session")
	}
	if wr.OriginalBranch != orig1 {
		t.Errorf("expected original branch %s recorded, got %s", orig1, wr.OriginalBranch)
	}
	if wr.OriginalSHA == "" {
		t.Error("expected original SHA to be recorded")
	}
}

func TestWorkStart_DirtyRepoBlocksBeforeAnyMutation(t *testing.T) {
	cfg, mgr := setupWorkWorkspace(t, 2)
	repo1 := filepath.Join(cfg.General.WorkDir, "repo1")
	repo2 := filepath.Join(cfg.General.WorkDir, "repo2")

	orig1 := currentBranch(t, repo1)
	makeDirty(t, repo2)

	ws, err := mgr.WorkStart("feat", "feat", Filter{All: true})
	if err == nil {
		t.Fatal("expected error for dirty repo")
	}
	if ws != nil {
		t.Error("expected nil session on failure")
	}
	if !strings.Contains(err.Error(), "repo2") {
		t.Errorf("expected error to name the dirty repo, got: %v", err)
	}

	// Validation happens before mutation: repo1 must be untouched, and the
	// work branch must not exist anywhere.
	if got := currentBranch(t, repo1); got != orig1 {
		t.Errorf("repo1 was mutated: on branch %s, want %s", got, orig1)
	}
	out := gitCmd(t, repo1, "branch", "--list", "feat")
	if strings.TrimSpace(out) != "" {
		t.Errorf("work branch was created in repo1 despite validation failure: %q", out)
	}
}

func TestWorkStart_MissingRepoBlocksBeforeAnyMutation(t *testing.T) {
	cfg, mgr := setupWorkWorkspace(t, 1)
	repo1 := filepath.Join(cfg.General.WorkDir, "repo1")
	orig1 := currentBranch(t, repo1)

	// Add a configured repo that does not exist on disk, after repo1.
	cfg.Repositories = append(cfg.Repositories, config.Repository{
		Name: "ghost",
		URL:  "https://example.com/ghost.git",
		Type: config.RepoTypeGit,
		Path: "ghost",
	})

	_, err := mgr.WorkStart("feat", "feat", Filter{All: true})
	if err == nil {
		t.Fatal("expected error for missing repo")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("expected error to name the missing repo, got: %v", err)
	}
	if got := currentBranch(t, repo1); got != orig1 {
		t.Errorf("repo1 was mutated: on branch %s, want %s", got, orig1)
	}
}

func TestWorkStart_RollsBackOnMidLoopFailure(t *testing.T) {
	cfg, mgr := setupWorkWorkspace(t, 2)
	repo1 := filepath.Join(cfg.General.WorkDir, "repo1")
	repo2 := filepath.Join(cfg.General.WorkDir, "repo2")

	orig1 := currentBranch(t, repo1)
	orig2 := currentBranch(t, repo2)

	// Make branch creation fail in repo2 only: a ref named feat/x blocks the
	// creation of a ref named feat.
	gitCmd(t, repo2, "branch", "feat/x")

	ws, err := mgr.WorkStart("feat", "feat", Filter{All: true})
	if err == nil {
		t.Fatal("expected WorkStart to fail")
	}
	if ws != nil {
		t.Error("expected nil session on failure")
	}
	if !strings.Contains(err.Error(), "repo2") {
		t.Errorf("expected error to name the failing repo, got: %v", err)
	}

	// repo1 was switched before repo2 failed: it must be rolled back.
	if got := currentBranch(t, repo1); got != orig1 {
		t.Errorf("repo1 stranded on %s, want rollback to %s", got, orig1)
	}
	if got := currentBranch(t, repo2); got != orig2 {
		t.Errorf("repo2 left on %s, want %s", got, orig2)
	}
}

func TestWorkEnd_RestoresAllRepos(t *testing.T) {
	cfg, mgr := setupWorkWorkspace(t, 2)
	repo1 := filepath.Join(cfg.General.WorkDir, "repo1")
	repo2 := filepath.Join(cfg.General.WorkDir, "repo2")

	orig1 := currentBranch(t, repo1)
	orig2 := currentBranch(t, repo2)

	ws, err := mgr.WorkStart("feat", "feat", Filter{All: true})
	if err != nil {
		t.Fatalf("WorkStart failed: %v", err)
	}

	if err := mgr.WorkEnd(ws, false); err != nil {
		t.Fatalf("WorkEnd failed: %v", err)
	}

	if got := currentBranch(t, repo1); got != orig1 {
		t.Errorf("repo1 on %s, want %s", got, orig1)
	}
	if got := currentBranch(t, repo2); got != orig2 {
		t.Errorf("repo2 on %s, want %s", got, orig2)
	}
}

func TestWorkEnd_ContinuesPastRestoreFailure(t *testing.T) {
	cfg, mgr := setupWorkWorkspace(t, 2)
	repo2 := filepath.Join(cfg.General.WorkDir, "repo2")

	orig2 := currentBranch(t, repo2)

	ws, err := mgr.WorkStart("feat", "feat", Filter{All: true})
	if err != nil {
		t.Fatalf("WorkStart failed: %v", err)
	}

	// Sabotage repo1's recorded original branch so its restore fails.
	wr, ok := ws.GetRepo("repo1")
	if !ok {
		t.Fatal("repo1 missing from session")
	}
	wr.OriginalBranch = "does-not-exist"

	err = mgr.WorkEnd(ws, false)
	if err == nil {
		t.Fatal("expected WorkEnd to report the restore failure")
	}
	if !strings.Contains(err.Error(), "repo1") {
		t.Errorf("expected error to name repo1, got: %v", err)
	}

	// repo2 must still have been restored despite repo1 failing first.
	if got := currentBranch(t, repo2); got != orig2 {
		t.Errorf("repo2 was not restored: on %s, want %s", got, orig2)
	}
}

func TestWorkEnd_DirtyRepoBlocksWithoutForce(t *testing.T) {
	cfg, mgr := setupWorkWorkspace(t, 1)
	repo1 := filepath.Join(cfg.General.WorkDir, "repo1")
	orig1 := currentBranch(t, repo1)

	ws, err := mgr.WorkStart("feat", "feat", Filter{All: true})
	if err != nil {
		t.Fatalf("WorkStart failed: %v", err)
	}

	makeDirty(t, repo1)

	err = mgr.WorkEnd(ws, false)
	if err == nil {
		t.Fatal("expected dirty repo to block WorkEnd")
	}
	if !strings.Contains(err.Error(), "repo1") {
		t.Errorf("expected error to name repo1, got: %v", err)
	}
	if got := currentBranch(t, repo1); got != "feat" {
		t.Errorf("blocked WorkEnd must not switch branches, repo1 on %s", got)
	}

	// With force, the restore proceeds.
	cleanRepo(t, repo1)
	if err := mgr.WorkEnd(ws, true); err != nil {
		t.Fatalf("forced WorkEnd failed: %v", err)
	}
	if got := currentBranch(t, repo1); got != orig1 {
		t.Errorf("repo1 on %s, want %s", got, orig1)
	}
}

func TestWorkRemove_DirtyRepoBlocks(t *testing.T) {
	cfg, mgr := setupWorkWorkspace(t, 1)
	repo1 := filepath.Join(cfg.General.WorkDir, "repo1")
	orig1 := currentBranch(t, repo1)

	ws, err := mgr.WorkStart("feat", "feat", Filter{All: true})
	if err != nil {
		t.Fatalf("WorkStart failed: %v", err)
	}

	makeDirty(t, repo1)

	err = mgr.WorkRemove(ws, "repo1")
	if err == nil {
		t.Fatal("expected dirty repo to block WorkRemove")
	}
	if !strings.Contains(err.Error(), "uncommitted changes") {
		t.Errorf("expected uncommitted-changes error, got: %v", err)
	}
	if got := currentBranch(t, repo1); got != "feat" {
		t.Errorf("blocked WorkRemove must not switch branches, repo1 on %s", got)
	}
	if !ws.HasRepo("repo1") {
		t.Error("repo must remain in session when removal is blocked")
	}

	// Once clean, removal restores the original branch.
	cleanRepo(t, repo1)
	if err := mgr.WorkRemove(ws, "repo1"); err != nil {
		t.Fatalf("WorkRemove failed: %v", err)
	}
	if got := currentBranch(t, repo1); got != orig1 {
		t.Errorf("repo1 on %s, want %s", got, orig1)
	}
	if ws.HasRepo("repo1") {
		t.Error("repo must be removed from session")
	}
}

func TestWorkAdd_JoinsSessionOnWorkBranch(t *testing.T) {
	cfg, mgr := setupWorkWorkspace(t, 2)
	repo2 := filepath.Join(cfg.General.WorkDir, "repo2")

	ws, err := mgr.WorkStart("feat", "feat", Filter{Names: []string{"repo1"}})
	if err != nil {
		t.Fatalf("WorkStart failed: %v", err)
	}
	if ws.HasRepo("repo2") {
		t.Fatal("repo2 must not be in session yet")
	}

	if err := mgr.WorkAdd(ws, "repo2"); err != nil {
		t.Fatalf("WorkAdd failed: %v", err)
	}
	if !ws.HasRepo("repo2") {
		t.Error("repo2 missing from session after WorkAdd")
	}
	if got := currentBranch(t, repo2); got != "feat" {
		t.Errorf("repo2 on %s, want feat", got)
	}
}

func TestWorkStart_SkipsHTTPRepos(t *testing.T) {
	cfg, mgr := setupWorkWorkspace(t, 1)
	cfg.Repositories = append(cfg.Repositories, config.Repository{
		Name: "http-dep",
		URL:  "https://example.com/file.tar.gz",
		Type: config.RepoTypeHTTP,
		Path: "http-dep",
	})

	ws, err := mgr.WorkStart("feat", "feat", Filter{All: true})
	if err != nil {
		t.Fatalf("WorkStart failed: %v", err)
	}
	if ws.HasRepo("http-dep") {
		t.Error("HTTP repos must be skipped")
	}
	if len(ws.Repos) != 1 {
		t.Errorf("expected 1 repo in session, got %d", len(ws.Repos))
	}
}

func TestWorkStart_OnlyHTTPReposFails(t *testing.T) {
	workDir := t.TempDir()
	cfg := &config.Config{
		General: config.GeneralConfig{WorkDir: workDir, DefaultBranch: "main"},
		Repositories: []config.Repository{
			{Name: "http-dep", URL: "https://example.com/f.tgz", Type: config.RepoTypeHTTP, Path: "http-dep"},
		},
	}
	mgr := NewRepositoryManager(cfg, WithInteractive(false))

	_, err := mgr.WorkStart("feat", "feat", Filter{All: true})
	if err == nil {
		t.Fatal("expected error when only HTTP repos match")
	}
	if !strings.Contains(err.Error(), "skipped HTTP repos") {
		t.Errorf("expected skipped-HTTP error, got: %v", err)
	}
}

func TestWorkSessionRoundTrip(t *testing.T) {
	cfg, mgr := setupWorkWorkspace(t, 1)

	ws, err := mgr.WorkStart("feat", "feat", Filter{All: true})
	if err != nil {
		t.Fatalf("WorkStart failed: %v", err)
	}

	// The session survives a save/load cycle with enough data for WorkEnd.
	path := filepath.Join(cfg.General.WorkDir, work.WorkFileName)
	if err := ws.Save(path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	loaded, err := work.Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded == nil || len(loaded.Repos) != 1 {
		t.Fatal("expected loaded session with 1 repo")
	}
	if loaded.Repos[0].AddedAt.After(time.Now()) {
		t.Error("bogus AddedAt timestamp")
	}

	if err := mgr.WorkEnd(loaded, false); err != nil {
		t.Fatalf("WorkEnd on loaded session failed: %v", err)
	}
}
