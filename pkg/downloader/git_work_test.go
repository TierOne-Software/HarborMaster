package downloader

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupRepoWithBareOrigin creates a bare origin repository and a working
// clone of it, configured with a test identity.
func setupRepoWithBareOrigin(t *testing.T) (workDir, bareDir string) {
	t.Helper()

	src := setupTestGitRepo(t)

	bareDir = filepath.Join(t.TempDir(), "origin.git")
	gitRun(t, t.TempDir(), "clone", "--bare", src, bareDir)

	workDir = filepath.Join(t.TempDir(), "work")
	gitRun(t, t.TempDir(), "clone", bareDir, workDir)
	gitRun(t, workDir, "config", "user.email", "test@test.com")
	gitRun(t, workDir, "config", "user.name", "Test User")
	gitRun(t, workDir, "config", "commit.gpgSign", "false")

	return workDir, bareDir
}

func TestCreateBranchAndCheckoutBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repoDir := setupTestGitRepo(t)
	defaultBranch := gitRun(t, repoDir, "rev-parse", "--abbrev-ref", "HEAD")

	if err := CreateBranch(repoDir, "feature-1"); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	branch, err := GetCurrentBranch(repoDir)
	if err != nil {
		t.Fatalf("GetCurrentBranch failed: %v", err)
	}
	if branch != "feature-1" {
		t.Errorf("expected current branch 'feature-1', got '%s'", branch)
	}

	if err := CheckoutBranch(repoDir, defaultBranch); err != nil {
		t.Fatalf("CheckoutBranch failed: %v", err)
	}
	branch, _ = GetCurrentBranch(repoDir)
	if branch != defaultBranch {
		t.Errorf("expected current branch '%s', got '%s'", defaultBranch, branch)
	}

	// Nonexistent branch must error.
	if err := CheckoutBranch(repoDir, "does-not-exist"); err == nil {
		t.Error("expected error checking out nonexistent branch")
	}

	// Option-like branch names must be rejected before reaching git.
	if err := CreateBranch(repoDir, "--evil"); err == nil {
		t.Error("expected error for option-like branch name in CreateBranch")
	}
	if err := CheckoutBranch(repoDir, "--evil"); err == nil {
		t.Error("expected error for option-like branch name in CheckoutBranch")
	}
}

func TestBranchExists(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repoDir := setupTestGitRepo(t)
	defaultBranch := gitRun(t, repoDir, "rev-parse", "--abbrev-ref", "HEAD")

	exists, err := BranchExists(repoDir, defaultBranch)
	if err != nil {
		t.Fatalf("BranchExists failed: %v", err)
	}
	if !exists {
		t.Errorf("expected branch '%s' to exist", defaultBranch)
	}

	exists, err = BranchExists(repoDir, "does-not-exist")
	if err != nil {
		t.Fatalf("BranchExists failed: %v", err)
	}
	if exists {
		t.Error("expected nonexistent branch to not exist")
	}

	// A tag with the same name must not count as a local branch.
	gitRun(t, repoDir, "tag", "shadow")
	exists, err = BranchExists(repoDir, "shadow")
	if err != nil {
		t.Fatalf("BranchExists failed: %v", err)
	}
	if exists {
		t.Error("expected tag 'shadow' to not be reported as a branch")
	}

	// Errors (e.g. not a git repository) must be propagated, not swallowed.
	if _, err := BranchExists(t.TempDir(), "main"); err == nil {
		t.Error("expected error for non-repository path")
	}
}

func TestStageAllAndCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repoDir := setupTestGitRepo(t)

	if err := os.WriteFile(filepath.Join(repoDir, "new.txt"), []byte("new"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	if err := StageAll(repoDir); err != nil {
		t.Fatalf("StageAll failed: %v", err)
	}

	sha, err := Commit(repoDir, "Add new file")
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}
	if len(sha) != 40 {
		t.Errorf("expected 40-char SHA, got %q", sha)
	}

	dirty, err := IsDirty(repoDir)
	if err != nil {
		t.Fatalf("IsDirty failed: %v", err)
	}
	if dirty {
		t.Error("expected clean repo after commit")
	}

	head := gitRun(t, repoDir, "rev-parse", "HEAD")
	if sha != head {
		t.Errorf("Commit returned %s, but HEAD is %s", sha, head)
	}
}

func TestPush(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	workDir, bareDir := setupRepoWithBareOrigin(t)

	if err := CreateBranch(workDir, "push-me"); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	addCommit(t, workDir, "pushed.txt", "content", "Commit to push")

	if err := Push(workDir, "push-me"); err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	// The branch must now exist in the bare origin.
	gitRun(t, bareDir, "show-ref", "--verify", "refs/heads/push-me")

	// Option-like branch names must be rejected.
	if err := Push(workDir, "--all"); err == nil {
		t.Error("expected error for option-like branch name")
	}
}

func TestPushWithUpstream(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	workDir, bareDir := setupRepoWithBareOrigin(t)

	if err := CreateBranch(workDir, "tracked"); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	addCommit(t, workDir, "tracked.txt", "content", "Commit to push")

	if err := PushWithUpstream(workDir, "tracked"); err != nil {
		t.Fatalf("PushWithUpstream failed: %v", err)
	}

	gitRun(t, bareDir, "show-ref", "--verify", "refs/heads/tracked")

	upstream := gitRun(t, workDir, "rev-parse", "--abbrev-ref", "tracked@{upstream}")
	if upstream != "origin/tracked" {
		t.Errorf("expected upstream 'origin/tracked', got '%s'", upstream)
	}
}

func TestRemoteBranchExists(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	workDir, bareDir := setupRepoWithBareOrigin(t)
	defaultBranch := gitRun(t, workDir, "rev-parse", "--abbrev-ref", "HEAD")

	// Create a namespaced branch directly on the origin.
	gitRun(t, bareDir, "branch", "bar/foo")

	exists, err := RemoteBranchExists(workDir, defaultBranch)
	if err != nil {
		t.Fatalf("RemoteBranchExists failed: %v", err)
	}
	if !exists {
		t.Errorf("expected remote branch '%s' to exist", defaultBranch)
	}

	exists, err = RemoteBranchExists(workDir, "bar/foo")
	if err != nil {
		t.Fatalf("RemoteBranchExists failed: %v", err)
	}
	if !exists {
		t.Error("expected remote branch 'bar/foo' to exist")
	}

	// "foo" is a path suffix of "bar/foo" and must NOT match.
	exists, err = RemoteBranchExists(workDir, "foo")
	if err != nil {
		t.Fatalf("RemoteBranchExists failed: %v", err)
	}
	if exists {
		t.Error("expected 'foo' to not match remote branch 'bar/foo'")
	}

	// A repository without an origin remote must produce an error.
	noOrigin := setupTestGitRepo(t)
	if _, err := RemoteBranchExists(noOrigin, "main"); err == nil {
		t.Error("expected error for repository without origin")
	}
}

func TestGetChangedFiles(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repoDir := setupTestGitRepo(t)

	// Clean repository reports no changes.
	changes, err := GetChangedFiles(repoDir)
	if err != nil {
		t.Fatalf("GetChangedFiles failed: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("expected no changes in clean repo, got %v", changes)
	}

	// Staged rename: porcelain reports `R  old -> new`.
	gitRun(t, repoDir, "mv", "README.md", "renamed.md")

	// Untracked file with a non-ASCII name: porcelain C-quotes it.
	if err := os.WriteFile(filepath.Join(repoDir, "täst.txt"), []byte("x"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// Untracked file with a space (not quoted by porcelain).
	if err := os.WriteFile(filepath.Join(repoDir, "has space.txt"), []byte("x"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	changes, err = GetChangedFiles(repoDir)
	if err != nil {
		t.Fatalf("GetChangedFiles failed: %v", err)
	}

	byPath := make(map[string]string, len(changes))
	for _, c := range changes {
		byPath[c.Path] = c.Status
	}

	if status, ok := byPath["renamed.md"]; !ok || status != "R" {
		t.Errorf("expected rename to report new path 'renamed.md' with status 'R', got %v", changes)
	}
	if _, ok := byPath["README.md -> renamed.md"]; ok {
		t.Errorf("rename was not parsed: %v", changes)
	}
	if status, ok := byPath["täst.txt"]; !ok || status != "??" {
		t.Errorf("expected C-quoted path 'täst.txt' with status '??', got %v", changes)
	}
	if status, ok := byPath["has space.txt"]; !ok || status != "??" {
		t.Errorf("expected path 'has space.txt' with status '??', got %v", changes)
	}
}

func TestParsePorcelainLine(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		want   FileChange
		wantOK bool
	}{
		{"modified", "M  file.txt", FileChange{Status: "M", Path: "file.txt"}, true},
		{"untracked", "?? file.txt", FileChange{Status: "??", Path: "file.txt"}, true},
		{"rename", "R  old.txt -> new.txt", FileChange{Status: "R", Path: "new.txt"}, true},
		{"rename with spaces", "R  old name.txt -> new name.txt", FileChange{Status: "R", Path: "new name.txt"}, true},
		{
			"quoted path with octal escapes",
			`?? "t\303\244st.txt"`,
			FileChange{Status: "??", Path: "täst.txt"},
			true,
		},
		{
			"quoted rename",
			`R  "old \"q\".txt" -> "new \"q\".txt"`,
			FileChange{Status: "R", Path: `new "q".txt`},
			true,
		},
		{
			"quoted path containing arrow",
			`?? "a -> b.txt"`,
			FileChange{Status: "??", Path: "a -> b.txt"},
			true,
		},
		{"too short", "M", FileChange{}, false},
		{"empty", "", FileChange{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parsePorcelainLine(tt.line)
			if ok != tt.wantOK {
				t.Fatalf("expected ok=%v, got %v", tt.wantOK, ok)
			}
			if ok && got != tt.want {
				t.Errorf("expected %+v, got %+v", tt.want, got)
			}
		})
	}
}

func TestGetHeadSHA(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repoDir := setupTestGitRepo(t)

	sha, err := GetHeadSHA(repoDir)
	if err != nil {
		t.Fatalf("GetHeadSHA failed: %v", err)
	}
	if len(sha) != 40 {
		t.Errorf("expected 40-char SHA, got %q", sha)
	}
	if want := gitRun(t, repoDir, "rev-parse", "HEAD"); sha != want {
		t.Errorf("expected %s, got %s", want, sha)
	}

	if _, err := GetHeadSHA(t.TempDir()); err == nil {
		t.Error("expected error for non-repository path")
	}
}

func TestPushErrorScrubsCredentials(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	t.Setenv("NO_PROXY", "*")
	t.Setenv("no_proxy", "*")
	t.Setenv("GIT_TERMINAL_PROMPT", "0")

	repoDir := setupTestGitRepo(t)
	// Origin with embedded credentials pointing at a closed local port.
	gitRun(t, repoDir, "remote", "add", "origin", "http://user:sekret@127.0.0.1:1/repo.git")

	branch := gitRun(t, repoDir, "rev-parse", "--abbrev-ref", "HEAD")
	err := Push(repoDir, branch)
	if err == nil {
		t.Fatal("expected push to fail")
	}
	if strings.Contains(err.Error(), "sekret") {
		t.Errorf("error message leaked credentials: %v", err)
	}
}
