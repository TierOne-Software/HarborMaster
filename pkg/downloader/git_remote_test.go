package downloader

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveRemoteRef(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	src := setupTestGitRepo(t)
	head := gitRun(t, src, "rev-parse", "HEAD")
	branch := gitRun(t, src, "rev-parse", "--abbrev-ref", "HEAD")

	sha, err := ResolveRemoteRef(src, "refs/heads/"+branch)
	if err != nil {
		t.Fatalf("ResolveRemoteRef failed: %v", err)
	}
	if sha != head {
		t.Errorf("expected %s, got %s", head, sha)
	}

	// An absent ref is an error, not an empty result.
	_, err = ResolveRemoteRef(src, "refs/heads/does-not-exist")
	if err == nil {
		t.Error("expected error for missing ref")
	} else if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}

	// Ref names that look like flags are rejected before git runs.
	if _, err := ResolveRemoteRef(src, "--upload-pack=evil"); err == nil {
		t.Error("expected error for option-like ref")
	}
}

func TestRemoteContains(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	src := setupTestGitRepo(t)
	clone := filepath.Join(t.TempDir(), "clone")
	gitRun(t, t.TempDir(), "clone", src, clone)
	gitRun(t, clone, "config", "user.email", "test@test.com")
	gitRun(t, clone, "config", "user.name", "Test User")
	gitRun(t, clone, "config", "commit.gpgSign", "false")

	// The cloned HEAD is on origin's branch tip: contained.
	head := gitRun(t, clone, "rev-parse", "HEAD")
	ok, err := RemoteContains(clone, head)
	if err != nil {
		t.Fatalf("RemoteContains failed: %v", err)
	}
	if !ok {
		t.Error("expected cloned HEAD to be contained in an origin ref")
	}

	// A local-only commit is not contained.
	local := addCommit(t, clone, "local.txt", "local", "local commit")
	ok, err = RemoteContains(clone, local)
	if err != nil {
		t.Fatalf("RemoteContains failed: %v", err)
	}
	if ok {
		t.Error("expected local-only commit to not be contained in any origin ref")
	}

	// After pushing, the same commit is contained (push updates the local
	// remote-tracking ref).
	gitRun(t, src, "config", "receive.denyCurrentBranch", "ignore")
	gitRun(t, clone, "push", "origin", "HEAD:refs/heads/published")
	ok, err = RemoteContains(clone, local)
	if err != nil {
		t.Fatalf("RemoteContains failed: %v", err)
	}
	if !ok {
		t.Error("expected pushed commit to be contained in an origin ref")
	}
}
