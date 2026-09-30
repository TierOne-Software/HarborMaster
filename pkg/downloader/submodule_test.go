package downloader

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// addSubmodule adds subSrc as a submodule at path in the super repository
// and commits the gitlink. file:// transport must be allowed explicitly.
func addSubmodule(t *testing.T, superSrc, subSrc, path string) {
	t.Helper()
	gitRun(t, superSrc, "-c", "protocol.file.allow=always",
		"submodule", "add", subSrc, path)
	gitRun(t, superSrc, "commit", "-m", "add submodule "+path)
}

// bumpSubmodule advances the submodule source, updates the superproject's
// gitlink to the new commit, and returns the new submodule SHA.
func bumpSubmodule(t *testing.T, superSrc, subSrc, path string) string {
	t.Helper()
	newSHA := addCommit(t, subSrc, "bump.txt", "bumped", "bump submodule")
	superSub := filepath.Join(superSrc, path)
	gitRun(t, superSub, "fetch", "origin")
	gitRun(t, superSub, "checkout", newSHA)
	gitRun(t, superSrc, "add", path)
	gitRun(t, superSrc, "commit", "-m", "bump "+path)
	return newSHA
}

// allowFileTransport permits git's file:// transport for submodule clones,
// which git refuses by default (CVE-2022-39253). Test-only: real submodule
// URLs use ssh/https.
func allowFileTransport(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "protocol.file.allow")
	t.Setenv("GIT_CONFIG_VALUE_0", "always")
}

func TestGitDownloader_Update_SubmodulesFollowGitlinks(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	allowFileTransport(t)

	subSrc := setupTestGitRepo(t)
	superSrc := setupTestGitRepo(t)
	addSubmodule(t, superSrc, subSrc, "sub")

	dest := filepath.Join(t.TempDir(), "clone")
	dl := NewGitDownloader(Options{Submodules: true, Shallow: false})

	if _, err := dl.Download(superSrc, dest); err != nil {
		t.Fatalf("clone failed: %v", err)
	}
	if got, err := GetHeadSHA(filepath.Join(dest, "sub")); err != nil || got == "" {
		t.Fatalf("submodule not materialized on clone: sha=%q err=%v", got, err)
	}

	// The superproject moves to a commit with a bumped gitlink.
	newSubSHA := bumpSubmodule(t, superSrc, subSrc, "sub")

	if _, err := dl.Update(dest); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	// The submodule checkout must follow the gitlink...
	if got, err := GetHeadSHA(filepath.Join(dest, "sub")); err != nil || got != newSubSHA {
		t.Errorf("submodule HEAD = %q (err %v), want %q", got, err, newSubSHA)
	}
	// ...and the parent must not be left phantom-dirty.
	if dirty, err := IsDirty(dest); err != nil || dirty {
		t.Errorf("parent dirty after update: %v (err %v)", dirty, err)
	}
	if mismatched, err := CheckSubmodules(dest); err != nil || len(mismatched) != 0 {
		t.Errorf("CheckSubmodules = %v (err %v), want none", mismatched, err)
	}
}

func TestCheckSubmodules_DetectsMismatch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	allowFileTransport(t)

	subSrc := setupTestGitRepo(t)
	superSrc := setupTestGitRepo(t)
	addSubmodule(t, superSrc, subSrc, "sub")

	dest := filepath.Join(t.TempDir(), "clone")
	dl := NewGitDownloader(Options{Submodules: true, Shallow: false})
	if _, err := dl.Download(superSrc, dest); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	// Move the submodule checkout off the recorded gitlink.
	sub := filepath.Join(dest, "sub")
	gitRun(t, sub, "checkout", "HEAD~0") // no-op detach, still clean
	if mismatched, _ := CheckSubmodules(dest); len(mismatched) != 0 {
		t.Fatalf("precondition failed: %v", mismatched)
	}
	bump := addCommit(t, subSrc, "other.txt", "other", "other commit")
	gitRun(t, sub, "fetch", "origin")
	gitRun(t, sub, "checkout", bump)

	mismatched, err := CheckSubmodules(dest)
	if err != nil {
		t.Fatalf("CheckSubmodules failed: %v", err)
	}
	if len(mismatched) != 1 || mismatched[0] != "sub" {
		t.Errorf("expected [sub], got %v", mismatched)
	}
}

func TestGitDownloader_Update_SubmodulesDisabledSkips(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	subSrc := setupTestGitRepo(t)
	superSrc := setupTestGitRepo(t)
	addSubmodule(t, superSrc, subSrc, "sub")
	newSubSHA := bumpSubmodule(t, superSrc, subSrc, "sub")

	// Clone without recursing, then update with submodules disabled:
	// the submodule checkout must stay empty/untouched and the update
	// must not fail.
	dest := filepath.Join(t.TempDir(), "clone")
	dl := NewGitDownloader(Options{Submodules: false, Shallow: false})
	if _, err := dl.Download(superSrc, dest); err != nil {
		t.Fatalf("clone failed: %v", err)
	}
	if _, err := dl.Update(dest); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if got, _ := GetHeadSHA(filepath.Join(dest, "sub")); got == newSubSHA {
		t.Error("submodule was updated despite submodules being disabled")
	}
}
