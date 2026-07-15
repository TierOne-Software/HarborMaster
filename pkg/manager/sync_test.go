package manager

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tierone/harbormaster/pkg/config"
	"github.com/tierone/harbormaster/pkg/lockfile"
)

// gitCmd runs a git command in dir and returns its trimmed output.
func gitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// addCommit writes a file, commits it, and returns the new HEAD SHA.
func addCommit(t *testing.T, dir, file, content string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", file, err)
	}
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "update "+file)
	return gitCmd(t, dir, "rev-parse", "HEAD")
}

func headSHA(t *testing.T, dir string) string {
	t.Helper()
	return gitCmd(t, dir, "rev-parse", "HEAD")
}

// newSyncConfig builds a config with the given repositories and a temp workdir.
func newSyncConfig(t *testing.T, repos ...config.Repository) *config.Config {
	t.Helper()
	return &config.Config{
		General: config.GeneralConfig{
			WorkDir:       t.TempDir(),
			DefaultBranch: "main",
			Timeout:       config.DefaultTimeout,
		},
		Git: config.GitConfig{
			ShallowClone: false,
			CloneDepth:   0,
		},
		Repositories: repos,
	}
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
}

func TestSync_MixedSuccessAndFailure(t *testing.T) {
	requireGit(t)

	good1 := setupTestGitRepo(t, "good1")
	good2 := setupTestGitRepo(t, "good2")
	badURL := filepath.Join(t.TempDir(), "does-not-exist")

	cfg := newSyncConfig(t,
		config.Repository{Name: "good1", URL: good1, Type: config.RepoTypeGit, Path: "good1"},
		config.Repository{Name: "bad", URL: badURL, Type: config.RepoTypeGit, Path: "bad"},
		config.Repository{Name: "good2", URL: good2, Type: config.RepoTypeGit, Path: "good2"},
	)

	lf := lockfile.New()
	mgr := NewRepositoryManager(cfg,
		WithLockFile(lf),
		WithInteractive(false),
	)

	result, err := mgr.Sync(Filter{All: true})
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	if result.TotalRepos != 3 {
		t.Errorf("expected 3 total repos, got %d", result.TotalRepos)
	}
	if result.SuccessCount != 2 {
		t.Errorf("expected 2 successes, got %d", result.SuccessCount)
	}
	if result.FailureCount != 1 {
		t.Errorf("expected 1 failure, got %d", result.FailureCount)
	}
	if !result.HasFailures() {
		t.Error("expected HasFailures to be true")
	}

	// One failure must not mask the others: per-repo results must be accurate.
	for _, r := range result.Results {
		switch r.RepoName {
		case "good1", "good2":
			if !r.Success {
				t.Errorf("expected %s to succeed, got error: %v", r.RepoName, r.Error)
			}
			if r.CommitSHA == "" {
				t.Errorf("expected %s to have a commit SHA", r.RepoName)
			}
		case "bad":
			if r.Success {
				t.Error("expected bad repo to fail")
			}
			if r.Error == nil {
				t.Error("expected bad repo result to carry an error")
			}
		}
	}

	// Lock file entries only for successes.
	if !lf.Has("good1") || !lf.Has("good2") {
		t.Error("expected lock file entries for successful repos")
	}
	if lf.Has("bad") {
		t.Error("must not record a lock entry for a failed repo")
	}
}

func TestSync_ConcurrencyOne(t *testing.T) {
	requireGit(t)

	good := setupTestGitRepo(t, "good")
	badURL := filepath.Join(t.TempDir(), "missing")

	cfg := newSyncConfig(t,
		config.Repository{Name: "bad", URL: badURL, Type: config.RepoTypeGit, Path: "bad"},
		config.Repository{Name: "good", URL: good, Type: config.RepoTypeGit, Path: "good"},
	)

	mgr := NewRepositoryManager(cfg,
		WithConcurrency(1),
		WithInteractive(false),
	)

	result, err := mgr.Sync(Filter{All: true})
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	if result.SuccessCount != 1 || result.FailureCount != 1 {
		t.Errorf("expected 1 success and 1 failure, got %d/%d", result.SuccessCount, result.FailureCount)
	}
}

func TestSync_RepeatedOnSameManager(t *testing.T) {
	requireGit(t)

	src := setupTestGitRepo(t, "src")
	cfg := newSyncConfig(t,
		config.Repository{Name: "repo", URL: src, Type: config.RepoTypeGit, Path: "repo"},
	)

	mgr := NewRepositoryManager(cfg, WithInteractive(false))

	first, err := mgr.Sync(Filter{All: true})
	if err != nil {
		t.Fatalf("first Sync failed: %v", err)
	}
	if first.FailureCount != 0 {
		t.Fatalf("first Sync had failures: %+v", first.FailedResults())
	}

	// Second Sync on the same manager must not panic (previously: double
	// close of the UI done channel) and must exercise the update path.
	second, err := mgr.Sync(Filter{All: true})
	if err != nil {
		t.Fatalf("second Sync failed: %v", err)
	}
	if second.FailureCount != 0 {
		t.Fatalf("second Sync had failures: %+v", second.FailedResults())
	}
	if second.SuccessCount != 1 {
		t.Errorf("expected 1 success on second sync, got %d", second.SuccessCount)
	}
}

func TestSync_LockedMode_ChecksOutLockedSHA(t *testing.T) {
	requireGit(t)

	src := setupTestGitRepo(t, "src")
	cfg := newSyncConfig(t,
		config.Repository{Name: "repo", URL: src, Type: config.RepoTypeGit, Path: "repo"},
	)
	repoPath := filepath.Join(cfg.General.WorkDir, "repo")

	lf := lockfile.New()

	// First: a normal sync records the current tip in the lock file.
	mgr := NewRepositoryManager(cfg, WithLockFile(lf), WithInteractive(false))
	result, err := mgr.Sync(Filter{All: true})
	if err != nil {
		t.Fatalf("initial Sync failed: %v", err)
	}
	if result.FailureCount != 0 {
		t.Fatalf("initial Sync had failures: %+v", result.FailedResults())
	}

	lockedSHA, ok := lf.GetResolvedSHA("repo")
	if !ok || lockedSHA == "" {
		t.Fatal("expected lock file to record a SHA")
	}

	// The source moves ahead.
	newTip := addCommit(t, src, "new.txt", "new content")
	if newTip == lockedSHA {
		t.Fatal("expected source tip to advance")
	}

	// Locked sync must keep the working tree at the locked SHA, not move it
	// to the new tip and then report a mismatch.
	lockedMgr := NewRepositoryManager(cfg,
		WithLockFile(lf),
		WithLocked(true),
		WithInteractive(false),
	)
	lockedResult, err := lockedMgr.Sync(Filter{All: true})
	if err != nil {
		t.Fatalf("locked Sync failed: %v", err)
	}
	if lockedResult.FailureCount != 0 {
		t.Fatalf("locked Sync had failures: %+v", lockedResult.FailedResults())
	}

	if got := headSHA(t, repoPath); got != lockedSHA {
		t.Errorf("locked sync left tree at %s, want locked SHA %s", got, lockedSHA)
	}

	// Locked mode must not rewrite the lock file.
	if sha, _ := lf.GetResolvedSHA("repo"); sha != lockedSHA {
		t.Errorf("locked sync must not update the lock file: got %s, want %s", sha, lockedSHA)
	}

	// Fresh clone in locked mode must also land on the locked SHA.
	if err := os.RemoveAll(repoPath); err != nil {
		t.Fatalf("failed to remove clone: %v", err)
	}
	freshResult, err := lockedMgr.Sync(Filter{All: true})
	if err != nil {
		t.Fatalf("locked fresh-clone Sync failed: %v", err)
	}
	if freshResult.FailureCount != 0 {
		t.Fatalf("locked fresh-clone Sync had failures: %+v", freshResult.FailedResults())
	}
	if got := headSHA(t, repoPath); got != lockedSHA {
		t.Errorf("locked fresh clone left tree at %s, want locked SHA %s", got, lockedSHA)
	}
}

func TestSync_LockedMode_MissingLockEntryFails(t *testing.T) {
	requireGit(t)

	src := setupTestGitRepo(t, "src")
	cfg := newSyncConfig(t,
		config.Repository{Name: "repo", URL: src, Type: config.RepoTypeGit, Path: "repo"},
	)

	mgr := NewRepositoryManager(cfg,
		WithLockFile(lockfile.New()),
		WithLocked(true),
		WithInteractive(false),
	)

	result, err := mgr.Sync(Filter{All: true})
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}
	if result.FailureCount != 1 {
		t.Fatalf("expected 1 failure, got %d", result.FailureCount)
	}
	failed := result.FailedResults()[0]
	if failed.Error == nil || !strings.Contains(failed.Error.Error(), "no lock entry") {
		t.Errorf("expected 'no lock entry' error, got: %v", failed.Error)
	}
}

func TestSync_LockedMode_CorruptShortSHADoesNotPanic(t *testing.T) {
	requireGit(t)

	src := setupTestGitRepo(t, "src")
	cfg := newSyncConfig(t,
		config.Repository{Name: "repo", URL: src, Type: config.RepoTypeGit, Path: "repo"},
	)

	// Simulate a corrupt lock file with a truncated SHA.
	lf := lockfile.New()
	lf.Update("repo", lockfile.LockEntry{ResolvedSHA: "abc"})

	mgr := NewRepositoryManager(cfg,
		WithLockFile(lf),
		WithLocked(true),
		WithInteractive(false),
	)

	// Must not panic on sha[:8]; the repo simply fails to sync.
	result, err := mgr.Sync(Filter{All: true})
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}
	if result.FailureCount != 1 {
		t.Errorf("expected the corrupt entry to fail the repo, got %d failures", result.FailureCount)
	}
}

func TestSyncOne_Failure(t *testing.T) {
	requireGit(t)

	badURL := filepath.Join(t.TempDir(), "missing")
	cfg := newSyncConfig(t,
		config.Repository{Name: "bad", URL: badURL, Type: config.RepoTypeGit, Path: "bad"},
	)

	lf := lockfile.New()
	mgr := NewRepositoryManager(cfg, WithLockFile(lf), WithInteractive(false))

	result, err := mgr.SyncOne("bad")
	if err != nil {
		t.Fatalf("SyncOne returned unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure result")
	}
	if result.Error == nil {
		t.Error("expected result to carry an error")
	}
	if lf.Has("bad") {
		t.Error("must not record a lock entry for a failed sync")
	}
}

func TestStatus_NoLockFileReportsNeedsUpdate(t *testing.T) {
	requireGit(t)

	src := setupTestGitRepo(t, "src")
	cfg := newSyncConfig(t,
		config.Repository{Name: "repo", URL: src, Type: config.RepoTypeGit, Path: "repo"},
	)

	// Clone into the workdir.
	gitCmd(t, ".", "clone", src, filepath.Join(cfg.General.WorkDir, "repo"))

	mgr := NewRepositoryManager(cfg) // no lock file
	statuses, err := mgr.Status(Filter{All: true})
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if !statuses[0].NeedsUpdate {
		t.Error("without a lock file, status must not claim the repo is up to date")
	}
}

func TestShortSHA(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", "(unknown)"},
		{"abc", "abc"},
		{"abcd1234", "abcd1234"},
		{"abcd1234ef567890", "abcd1234"},
	}
	for _, tt := range tests {
		if got := shortSHA(tt.in); got != tt.want {
			t.Errorf("shortSHA(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
