package manager

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tierone/harbormaster/pkg/config"
	"github.com/tierone/harbormaster/pkg/downloader"
	"github.com/tierone/harbormaster/pkg/lockfile"
)

// TestSync_LockedMode_SubmodulesVerified exercises the locked-sync path over
// a superproject with a submodule: the checkout must stay at the locked
// commit, submodules must follow the locked gitlinks, and the submodule
// verification must pass.
func TestSync_LockedMode_SubmodulesVerified(t *testing.T) {
	requireGit(t)
	// Submodule clones from local paths need the file transport explicitly
	// allowed (git refuses it by default, CVE-2022-39253).
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "protocol.file.allow")
	t.Setenv("GIT_CONFIG_VALUE_0", "always")

	subSrc := setupTestGitRepo(t, "subsrc")
	superSrc := setupTestGitRepo(t, "supersrc")
	gitCmd(t, superSrc, "-c", "protocol.file.allow=always", "submodule", "add", subSrc, "sub")
	gitCmd(t, superSrc, "commit", "-m", "add submodule")
	subSHA1 := headSHA(t, subSrc)

	sub := true
	cfg := newSyncConfig(t,
		config.Repository{Name: "repo", URL: superSrc, Type: config.RepoTypeGit, Path: "repo", Submodules: &sub},
	)
	repoPath := filepath.Join(cfg.General.WorkDir, "repo")

	lf := lockfile.New()
	mgr := NewRepositoryManager(cfg, WithLockFile(lf), WithInteractive(false))
	result, err := mgr.Sync(Filter{All: true})
	if err != nil || result.FailureCount != 0 {
		t.Fatalf("initial sync failed: %v %+v", err, result.FailedResults())
	}
	lockedSHA, _ := lf.GetResolvedSHA("repo")

	// The source advances across a gitlink bump.
	newSubSHA := addCommit(t, subSrc, "bump.txt", "bumped")
	superSub := filepath.Join(superSrc, "sub")
	gitCmd(t, superSub, "fetch", "origin")
	gitCmd(t, superSub, "checkout", newSubSHA)
	gitCmd(t, superSrc, "add", "sub")
	gitCmd(t, superSrc, "commit", "-m", "bump sub")

	// Locked sync: superproject stays at the locked commit, submodule at the
	// locked gitlink, and verification passes.
	lockedMgr := NewRepositoryManager(cfg, WithLockFile(lf), WithLocked(true), WithInteractive(false))
	lockedResult, err := lockedMgr.Sync(Filter{All: true})
	if err != nil || lockedResult.FailureCount != 0 {
		t.Fatalf("locked sync failed: %v %+v", err, lockedResult.FailedResults())
	}
	if got := headSHA(t, repoPath); got != lockedSHA {
		t.Errorf("locked sync moved HEAD to %s, want %s", got, lockedSHA)
	}
	if got := headSHA(t, filepath.Join(repoPath, "sub")); got != subSHA1 {
		t.Errorf("locked sync submodule at %s, want locked gitlink %s", got, subSHA1)
	}

	// A fresh clone directly in locked mode must land the submodule on the
	// locked gitlink too: the clone initializes submodules for the remote
	// tip, which has moved past the pin.
	if err := os.RemoveAll(repoPath); err != nil {
		t.Fatalf("failed to remove clone: %v", err)
	}
	freshResult, err := lockedMgr.Sync(Filter{All: true})
	if err != nil || freshResult.FailureCount != 0 {
		t.Fatalf("locked fresh-clone sync failed: %v %+v", err, freshResult.FailedResults())
	}
	if got := headSHA(t, filepath.Join(repoPath, "sub")); got != subSHA1 {
		t.Errorf("locked fresh clone submodule at %s, want locked gitlink %s", got, subSHA1)
	}

	// Normal sync: the superproject moves and the submodule follows.
	result, err = mgr.Sync(Filter{All: true})
	if err != nil || result.FailureCount != 0 {
		t.Fatalf("second sync failed: %v %+v", err, result.FailedResults())
	}
	if got := headSHA(t, filepath.Join(repoPath, "sub")); got != newSubSHA {
		t.Errorf("after normal sync submodule at %s, want %s", got, newSubSHA)
	}
	if mismatched, err := downloader.CheckSubmodules(repoPath); err != nil || len(mismatched) != 0 {
		t.Errorf("CheckSubmodules after sync = %v (err %v), want none", mismatched, err)
	}
}
