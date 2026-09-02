package manager

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/tierone/harbormaster/pkg/config"
	"github.com/tierone/harbormaster/pkg/lockfile"
)

// testRepoBranch returns the initial branch of a freshly created test repo.
func testRepoBranch(t *testing.T, dir string) string {
	t.Helper()
	return gitCmd(t, dir, "rev-parse", "--abbrev-ref", "HEAD")
}

func findChange(changes []LockChange, name string) *LockChange {
	for i := range changes {
		if changes[i].Name == name {
			return &changes[i]
		}
	}
	return nil
}

func TestUpdateLockToRemote(t *testing.T) {
	requireGit(t)

	src := setupTestGitRepo(t, "src")
	branch := testRepoBranch(t, src)
	cfg := newSyncConfig(t,
		config.Repository{Name: "repo", URL: src, Type: config.RepoTypeGit, Path: "repo", Branch: branch},
	)
	repoPath := filepath.Join(cfg.General.WorkDir, "repo")

	lf := lockfile.New()
	mgr := NewRepositoryManager(cfg, WithLockFile(lf), WithInteractive(false))
	if _, err := mgr.Sync(Filter{All: true}); err != nil {
		t.Fatalf("initial Sync failed: %v", err)
	}
	lockedSHA, _ := lf.GetResolvedSHA("repo")

	// The source moves ahead.
	newTip := addCommit(t, src, "new.txt", "new content")

	changes, err := mgr.UpdateLockToRemote(Filter{All: true})
	if err != nil {
		t.Fatalf("UpdateLockToRemote failed: %v", err)
	}
	c := findChange(changes, "repo")
	if c == nil {
		t.Fatal("expected a change record for repo")
	}
	if c.Error != nil || c.Skipped {
		t.Fatalf("unexpected change state: %+v", c)
	}
	if !c.Changed || c.OldSHA != lockedSHA || c.NewSHA != newTip {
		t.Errorf("expected %s -> %s, got %+v", lockedSHA, newTip, c)
	}

	// Lock file updated...
	if sha, _ := lf.GetResolvedSHA("repo"); sha != newTip {
		t.Errorf("lock file has %s, want %s", sha, newTip)
	}
	// ...but the checkout must be untouched.
	if got := headSHA(t, repoPath); got != lockedSHA {
		t.Errorf("checkout moved to %s, want untouched %s", got, lockedSHA)
	}

	// A second run reports no change.
	changes, err = mgr.UpdateLockToRemote(Filter{All: true})
	if err != nil {
		t.Fatalf("second UpdateLockToRemote failed: %v", err)
	}
	if c := findChange(changes, "repo"); c.Changed {
		t.Errorf("expected unchanged on second run, got %+v", c)
	}
}

func TestUpdateLockToRemote_SkipsPinnedAndHTTP(t *testing.T) {
	requireGit(t)

	src := setupTestGitRepo(t, "src")
	pinned := headSHA(t, src)
	cfg := newSyncConfig(t,
		config.Repository{Name: "pinned", URL: src, Type: config.RepoTypeGit, Path: "pinned", Commit: pinned},
		config.Repository{Name: "asset", URL: "https://example.com/a.bin", Type: config.RepoTypeHTTP, Path: "asset"},
	)

	lf := lockfile.New()
	mgr := NewRepositoryManager(cfg, WithLockFile(lf), WithInteractive(false))

	changes, err := mgr.UpdateLockToRemote(Filter{All: true})
	if err != nil {
		t.Fatalf("UpdateLockToRemote failed: %v", err)
	}
	for _, name := range []string{"pinned", "asset"} {
		c := findChange(changes, name)
		if c == nil || !c.Skipped {
			t.Errorf("expected %s to be skipped, got %+v", name, c)
		}
	}
	if lf.Len() != 0 {
		t.Errorf("no lock entries should be written, got %d", lf.Len())
	}
}

func TestUpdateLockToRemote_UnknownBranchFails(t *testing.T) {
	requireGit(t)

	src := setupTestGitRepo(t, "src")
	cfg := newSyncConfig(t,
		config.Repository{Name: "repo", URL: src, Type: config.RepoTypeGit, Path: "repo", Branch: "no-such-branch"},
	)

	lf := lockfile.New()
	mgr := NewRepositoryManager(cfg, WithLockFile(lf), WithInteractive(false))

	changes, err := mgr.UpdateLockToRemote(Filter{All: true})
	if err != nil {
		t.Fatalf("UpdateLockToRemote failed: %v", err)
	}
	c := findChange(changes, "repo")
	if c == nil || c.Error == nil {
		t.Fatalf("expected an error for the unknown branch, got %+v", c)
	}
	if !strings.Contains(c.Error.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", c.Error)
	}
	if lf.Has("repo") {
		t.Error("must not write a lock entry for a failed repo")
	}
}

func TestUpdateLockToLocal(t *testing.T) {
	requireGit(t)

	src := setupTestGitRepo(t, "src")
	branch := testRepoBranch(t, src)
	cfg := newSyncConfig(t,
		config.Repository{Name: "repo", URL: src, Type: config.RepoTypeGit, Path: "repo", Branch: branch},
	)
	repoPath := filepath.Join(cfg.General.WorkDir, "repo")

	lf := lockfile.New()
	mgr := NewRepositoryManager(cfg, WithLockFile(lf), WithInteractive(false))
	if _, err := mgr.Sync(Filter{All: true}); err != nil {
		t.Fatalf("initial Sync failed: %v", err)
	}
	lockedSHA, _ := lf.GetResolvedSHA("repo")

	// Commit locally without pushing.
	gitCmd(t, repoPath, "config", "user.email", "test@test.com")
	gitCmd(t, repoPath, "config", "user.name", "Test User")
	gitCmd(t, repoPath, "config", "commit.gpgSign", "false")
	localSHA := addCommit(t, repoPath, "local.txt", "local content")

	// Unpushed HEAD: refused without force.
	changes, err := mgr.UpdateLockToLocal(Filter{All: true}, false)
	if err != nil {
		t.Fatalf("UpdateLockToLocal failed: %v", err)
	}
	c := findChange(changes, "repo")
	if c == nil || c.Error == nil {
		t.Fatalf("expected refusal for unpushed HEAD, got %+v", c)
	}
	if !strings.Contains(c.Error.Error(), "--force") {
		t.Errorf("expected error to mention --force, got: %v", c.Error)
	}
	if sha, _ := lf.GetResolvedSHA("repo"); sha != lockedSHA {
		t.Error("lock file must not change when adopt is refused")
	}

	// With force the entry is written and carries a warning.
	changes, err = mgr.UpdateLockToLocal(Filter{All: true}, true)
	if err != nil {
		t.Fatalf("UpdateLockToLocal --force failed: %v", err)
	}
	c = findChange(changes, "repo")
	if c.Error != nil || !c.Changed || c.NewSHA != localSHA || c.OldSHA != lockedSHA {
		t.Errorf("unexpected forced adopt result: %+v", c)
	}
	if c.Warning == "" {
		t.Error("expected a warning when adopting an unpushed HEAD")
	}
	if sha, _ := lf.GetResolvedSHA("repo"); sha != localSHA {
		t.Errorf("lock file has %s, want %s", sha, localSHA)
	}

	// After pushing, adopt succeeds without force and without warning.
	gitCmd(t, src, "config", "receive.denyCurrentBranch", "ignore")
	gitCmd(t, repoPath, "push", "origin", "HEAD:refs/heads/"+branch)
	changes, err = mgr.UpdateLockToLocal(Filter{All: true}, false)
	if err != nil {
		t.Fatalf("UpdateLockToLocal after push failed: %v", err)
	}
	c = findChange(changes, "repo")
	if c.Error != nil || c.Warning != "" {
		t.Errorf("expected clean adopt after push, got %+v", c)
	}
	if c.Changed {
		t.Error("expected unchanged: lock already at local HEAD")
	}
}

func TestUpdateLockToLocal_MissingRepo(t *testing.T) {
	requireGit(t)

	src := setupTestGitRepo(t, "src")
	cfg := newSyncConfig(t,
		config.Repository{Name: "repo", URL: src, Type: config.RepoTypeGit, Path: "repo"},
	)

	mgr := NewRepositoryManager(cfg, WithLockFile(lockfile.New()), WithInteractive(false))
	changes, err := mgr.UpdateLockToLocal(Filter{All: true}, false)
	if err != nil {
		t.Fatalf("UpdateLockToLocal failed: %v", err)
	}
	c := findChange(changes, "repo")
	if c == nil || c.Error == nil {
		t.Fatal("expected an error for a missing checkout")
	}
	if !strings.Contains(c.Error.Error(), "hm sync") {
		t.Errorf("expected error to point at 'hm sync', got: %v", c.Error)
	}
}
