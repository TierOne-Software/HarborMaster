package downloader

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tierone/harbormaster/pkg/types"
)

// gitRun runs a git command in dir, failing the test on error, and returns
// the trimmed output.
func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to run git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// addCommit creates a file and commits it, returning the new HEAD SHA.
func addCommit(t *testing.T, repoDir, filename, content, message string) string {
	t.Helper()

	if err := os.WriteFile(filepath.Join(repoDir, filename), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", filename, err)
	}
	gitRun(t, repoDir, "add", ".")
	gitRun(t, repoDir, "commit", "-m", message)
	return gitRun(t, repoDir, "rev-parse", "HEAD")
}

// fileURL converts a local path into a file:// URL so git uses the real
// transport (required for shallow clones of local repositories).
func fileURL(path string) string {
	return "file://" + path
}

// setupTestGitRepo creates a temporary git repository for testing
func setupTestGitRepo(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "test-repo")

	// Initialize repo
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
	if err := os.WriteFile(testFile, []byte("# Test Repo"), 0644); err != nil {
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

func TestGitDownloader_Type(t *testing.T) {
	dl := NewGitDownloader(DefaultOptions())
	if dl.Type() != "git" {
		t.Errorf("expected type 'git', got '%s'", dl.Type())
	}
}

func TestGitDownloader_Download(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	sourceRepo := setupTestGitRepo(t)
	destDir := filepath.Join(t.TempDir(), "cloned")

	dl := NewGitDownloader(Options{
		Shallow: false,
		Timeout: 30 * time.Second,
	})

	sha, err := dl.Download(sourceRepo, destDir)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	// Verify clone
	if _, err := os.Stat(filepath.Join(destDir, ".git")); err != nil {
		t.Error("expected .git directory")
	}

	if _, err := os.Stat(filepath.Join(destDir, "README.md")); err != nil {
		t.Error("expected README.md")
	}

	if sha == "" {
		t.Error("expected SHA to be returned")
	}
}

func TestGitDownloader_DownloadWithProgress(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	sourceRepo := setupTestGitRepo(t)
	destDir := filepath.Join(t.TempDir(), "cloned")

	dl := NewGitDownloader(Options{
		Shallow: false,
		Timeout: 30 * time.Second,
	})

	_, progressCh, err := dl.DownloadWithProgress(sourceRepo, destDir)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	// Consume progress updates
	var updates []types.ProgressUpdate
	for update := range progressCh {
		updates = append(updates, update)
	}

	if len(updates) == 0 {
		t.Fatal("expected at least one progress update")
	}

	// Last update must be a successful terminal update carrying the SHA.
	lastUpdate := updates[len(updates)-1]
	if lastUpdate.Phase != types.PhaseComplete {
		t.Errorf("expected final phase %s, got %s (error: %v)",
			types.PhaseComplete, lastUpdate.Phase, lastUpdate.Error)
	}
	if len(lastUpdate.Message) != 40 {
		t.Errorf("expected final message to be a 40-char SHA, got %q", lastUpdate.Message)
	}

	// Verify clone
	if _, err := os.Stat(filepath.Join(destDir, ".git")); err != nil {
		t.Error("expected .git directory after clone")
	}
}

func TestGitDownloader_GetCurrentRef(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repoDir := setupTestGitRepo(t)

	dl := NewGitDownloader(DefaultOptions())
	sha, err := dl.GetCurrentRef(repoDir)
	if err != nil {
		t.Fatalf("GetCurrentRef failed: %v", err)
	}

	if sha == "" {
		t.Error("expected SHA to be returned")
	}

	// SHA should be 40 characters (full SHA-1)
	if len(sha) != 40 {
		t.Errorf("expected 40-char SHA, got %d chars: %s", len(sha), sha)
	}
}

func TestGitDownloader_Update(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	sourceRepo := setupTestGitRepo(t)
	destDir := filepath.Join(t.TempDir(), "cloned")

	dl := NewGitDownloader(Options{
		Shallow: false,
		Timeout: 30 * time.Second,
	})

	// Initial clone
	_, err := dl.Download(sourceRepo, destDir)
	if err != nil {
		t.Fatalf("initial download failed: %v", err)
	}

	// Add another commit to source
	newSHA := addCommit(t, sourceRepo, "new-file.txt", "new content", "Second commit")

	// Update the clone. Even with no ref configured, Update must advance
	// HEAD to the new upstream tip instead of reporting the stale SHA.
	sha, err := dl.Update(destDir)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	if sha != newSHA {
		t.Errorf("expected HEAD to advance to %s, got %s", newSHA, sha)
	}
}

func TestIsGitRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repoDir := setupTestGitRepo(t)

	if !IsGitRepository(repoDir) {
		t.Error("expected IsGitRepository to return true for valid repo")
	}

	notRepo := t.TempDir()
	if IsGitRepository(notRepo) {
		t.Error("expected IsGitRepository to return false for non-repo")
	}
}

func TestGetRemoteURL(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Create a repo and add a remote
	repoDir := setupTestGitRepo(t)

	// Add origin remote
	cmd := exec.Command("git", "remote", "add", "origin", "https://github.com/test/repo.git")
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to add remote: %v\n%s", err, out)
	}

	url, err := GetRemoteURL(repoDir)
	if err != nil {
		t.Fatalf("GetRemoteURL failed: %v", err)
	}

	if url != "https://github.com/test/repo.git" {
		t.Errorf("expected 'https://github.com/test/repo.git', got '%s'", url)
	}
}

func TestIsDirty(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repoDir := setupTestGitRepo(t)

	// Clean state
	dirty, err := IsDirty(repoDir)
	if err != nil {
		t.Fatalf("IsDirty failed: %v", err)
	}
	if dirty {
		t.Error("expected clean repo to not be dirty")
	}

	// Make it dirty
	testFile := filepath.Join(repoDir, "dirty.txt")
	if err := os.WriteFile(testFile, []byte("dirty"), 0644); err != nil {
		t.Fatalf("failed to create dirty file: %v", err)
	}

	dirty, err = IsDirty(repoDir)
	if err != nil {
		t.Fatalf("IsDirty failed: %v", err)
	}
	if !dirty {
		t.Error("expected repo to be dirty after adding file")
	}
}

func TestGetCurrentBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repoDir := setupTestGitRepo(t)

	branch, err := GetCurrentBranch(repoDir)
	if err != nil {
		t.Fatalf("GetCurrentBranch failed: %v", err)
	}

	// Default branch might be "main" or "master"
	if branch != "main" && branch != "master" {
		t.Errorf("expected branch 'main' or 'master', got '%s'", branch)
	}
}

func TestExists(t *testing.T) {
	existingDir := t.TempDir()
	if !Exists(existingDir) {
		t.Error("expected Exists to return true for existing dir")
	}

	if Exists("/nonexistent/path") {
		t.Error("expected Exists to return false for nonexistent path")
	}
}

func TestExtractPercentage(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"Receiving objects: 50%", 50},
		{"Resolving deltas: 100%", 100},
		{"Compressing objects:  25%", 25},
		{"No percentage here", -1},
		{"", -1},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := extractPercentage(tt.input)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestGitDownloader_Download_PinnedCommitShallow(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	sourceRepo := setupTestGitRepo(t)
	firstSHA := gitRun(t, sourceRepo, "rev-parse", "HEAD")
	secondSHA := addCommit(t, sourceRepo, "second.txt", "second", "Second commit")

	destDir := filepath.Join(t.TempDir(), "cloned")

	// Shallow defaults must not break checking out a pinned commit that is
	// not the branch tip.
	dl := NewGitDownloader(Options{
		Commit:  firstSHA,
		Shallow: true,
		Depth:   1,
		Timeout: 30 * time.Second,
	})

	sha, err := dl.Download(fileURL(sourceRepo), destDir)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	if sha != firstSHA {
		t.Errorf("expected HEAD at pinned commit %s, got %s", firstSHA, sha)
	}
	if sha == secondSHA {
		t.Error("HEAD should not be at the branch tip")
	}
}

func TestGitDownloader_Update_PinnedCommitOnShallowSingleBranchClone(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	sourceRepo := setupTestGitRepo(t)
	firstSHA := gitRun(t, sourceRepo, "rev-parse", "HEAD")
	addCommit(t, sourceRepo, "second.txt", "second", "Second commit")

	// Simulate a pre-existing shallow, single-branch clone that does not
	// contain the pinned commit.
	destDir := filepath.Join(t.TempDir(), "cloned")
	gitRun(t, t.TempDir(), "clone", "--depth", "1", "--single-branch", fileURL(sourceRepo), destDir)

	// This mirrors the `sync --locked` contract: Options.Commit is set to a
	// locked SHA that is not the branch tip.
	dl := NewGitDownloader(Options{
		Commit:  firstSHA,
		Shallow: true,
		Depth:   1,
		Timeout: 30 * time.Second,
	})

	sha, err := dl.Update(destDir)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	if sha != firstSHA {
		t.Errorf("expected HEAD at pinned commit %s, got %s", firstSHA, sha)
	}
}

func TestGitDownloader_Update_BranchSwitchAfterSingleBranchClone(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	sourceRepo := setupTestGitRepo(t)
	defaultBranch := gitRun(t, sourceRepo, "rev-parse", "--abbrev-ref", "HEAD")

	// Create a feature branch with an extra commit, then go back.
	gitRun(t, sourceRepo, "checkout", "-b", "feature")
	featureSHA := addCommit(t, sourceRepo, "feature.txt", "feature", "Feature commit")
	gitRun(t, sourceRepo, "checkout", defaultBranch)

	// Pre-existing single-branch clone of the default branch.
	destDir := filepath.Join(t.TempDir(), "cloned")
	gitRun(t, t.TempDir(), "clone", "--depth", "1", "--single-branch",
		"--branch", defaultBranch, fileURL(sourceRepo), destDir)

	dl := NewGitDownloader(Options{
		Branch:  "feature",
		Shallow: true,
		Depth:   1,
		Timeout: 30 * time.Second,
	})

	sha, err := dl.Update(destDir)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	if sha != featureSHA {
		t.Errorf("expected HEAD at feature tip %s, got %s", featureSHA, sha)
	}
}

func TestGitDownloader_Download_TagShallow(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	sourceRepo := setupTestGitRepo(t)
	firstSHA := gitRun(t, sourceRepo, "rev-parse", "HEAD")
	gitRun(t, sourceRepo, "tag", "v1.0", firstSHA)
	addCommit(t, sourceRepo, "second.txt", "second", "Second commit")

	destDir := filepath.Join(t.TempDir(), "cloned")

	dl := NewGitDownloader(Options{
		Tag:     "v1.0",
		Shallow: true,
		Depth:   1,
		Timeout: 30 * time.Second,
	})

	sha, err := dl.Download(fileURL(sourceRepo), destDir)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	if sha != firstSHA {
		t.Errorf("expected HEAD at tagged commit %s, got %s", firstSHA, sha)
	}
}

func TestGitDownloader_DownloadWithProgress_ChecksOutBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	sourceRepo := setupTestGitRepo(t)
	defaultBranch := gitRun(t, sourceRepo, "rev-parse", "--abbrev-ref", "HEAD")
	gitRun(t, sourceRepo, "checkout", "-b", "feature")
	featureSHA := addCommit(t, sourceRepo, "feature.txt", "feature", "Feature commit")
	gitRun(t, sourceRepo, "checkout", defaultBranch)

	destDir := filepath.Join(t.TempDir(), "cloned")

	dl := NewGitDownloader(Options{
		Branch:  "feature",
		Timeout: 30 * time.Second,
	})

	_, progressCh, err := dl.DownloadWithProgress(fileURL(sourceRepo), destDir)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	var lastUpdate types.ProgressUpdate
	for update := range progressCh {
		lastUpdate = update
	}

	if lastUpdate.Phase != types.PhaseComplete {
		t.Fatalf("expected final phase %s, got %s (error: %v)",
			types.PhaseComplete, lastUpdate.Phase, lastUpdate.Error)
	}
	if lastUpdate.Message != featureSHA {
		t.Errorf("expected checked out SHA %s, got %s", featureSHA, lastUpdate.Message)
	}
}

func TestGitDownloader_Download_Timeout(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	sourceRepo := setupTestGitRepo(t)
	destDir := filepath.Join(t.TempDir(), "cloned")

	dl := NewGitDownloader(Options{
		Timeout: 1 * time.Nanosecond,
	})

	_, err := dl.Download(sourceRepo, destDir)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected 'timed out' in error, got: %v", err)
	}
}

func TestGitDownloader_Download_ScrubsCredentialsFromErrors(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Ensure git talks to the (closed) local port directly.
	t.Setenv("NO_PROXY", "*")
	t.Setenv("no_proxy", "*")
	t.Setenv("GIT_TERMINAL_PROMPT", "0")

	destDir := filepath.Join(t.TempDir(), "cloned")

	dl := NewGitDownloader(Options{Timeout: 30 * time.Second})

	// Port 1 is refused immediately; no external network is contacted.
	_, err := dl.Download("http://user:sekret@127.0.0.1:1/repo.git", destDir)
	if err == nil {
		t.Fatal("expected clone to fail")
	}
	if strings.Contains(err.Error(), "sekret") {
		t.Errorf("error message leaked credentials: %v", err)
	}
}

func TestGitDownloader_RejectsOptionInjectionInRefs(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	sourceRepo := setupTestGitRepo(t)
	destDir := filepath.Join(t.TempDir(), "cloned")

	dl := NewGitDownloader(Options{
		Commit:  "--upload-pack=/bin/true",
		Timeout: 30 * time.Second,
	})

	_, err := dl.Download(sourceRepo, destDir)
	if err == nil {
		t.Fatal("expected error for option-like ref")
	}
	if !strings.Contains(err.Error(), "invalid git ref") {
		t.Errorf("expected 'invalid git ref' error, got: %v", err)
	}
}

func TestScrubCredentials(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "user and token",
			input:    "fatal: unable to access 'https://user:token123@github.com/x/y.git/'",
			expected: "fatal: unable to access 'https://***@github.com/x/y.git/'",
		},
		{
			name:     "token only",
			input:    "https://ghp_secret@github.com/x/y.git",
			expected: "https://***@github.com/x/y.git",
		},
		{
			name:     "no credentials",
			input:    "https://github.com/x/y.git",
			expected: "https://github.com/x/y.git",
		},
		{
			name:     "scp-like syntax untouched",
			input:    "git@github.com:x/y.git",
			expected: "git@github.com:x/y.git",
		},
		{
			name:     "multiple URLs",
			input:    "http://a:b@x.com and ssh://c:d@y.com",
			expected: "http://***@x.com and ssh://***@y.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scrubCredentials(tt.input); got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestScanGitProgress(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "carriage returns and newlines",
			input:    "Receiving objects: 50%\rReceiving objects: 100%\nResolving deltas: 100%\n",
			expected: []string{"Receiving objects: 50%", "Receiving objects: 100%", "Resolving deltas: 100%"},
		},
		{
			name:     "trailing data without terminator",
			input:    "line one\nline two",
			expected: []string{"line one", "line two"},
		},
		{
			name:     "empty input",
			input:    "",
			expected: nil,
		},
		{
			name:     "consecutive separators produce empty tokens",
			input:    "a\r\nb",
			expected: []string{"a", "", "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scanner := bufio.NewScanner(strings.NewReader(tt.input))
			scanner.Split(scanGitProgress)

			var got []string
			for scanner.Scan() {
				got = append(got, scanner.Text())
			}
			if err := scanner.Err(); err != nil {
				t.Fatalf("scanner error: %v", err)
			}

			if len(got) != len(tt.expected) {
				t.Fatalf("expected %d tokens %q, got %d tokens %q",
					len(tt.expected), tt.expected, len(got), got)
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("token %d: expected %q, got %q", i, tt.expected[i], got[i])
				}
			}
		})
	}
}

func TestSendTerminal_DoesNotBlockWhenChannelFull(t *testing.T) {
	progress := make(chan types.ProgressUpdate, 2)

	// Fill the buffer so a plain send would block forever.
	progress <- types.ProgressUpdate{Phase: types.PhaseFetching, Message: "one"}
	progress <- types.ProgressUpdate{Phase: types.PhaseFetching, Message: "two"}

	done := make(chan struct{})
	go func() {
		sendTerminal(progress, types.ProgressUpdate{Phase: types.PhaseComplete, Message: "sha"})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("sendTerminal blocked on a full channel")
	}
	close(progress)

	// The terminal update must be retrievable by a consumer.
	found := false
	for update := range progress {
		if update.Phase == types.PhaseComplete && update.Message == "sha" {
			found = true
		}
	}
	if !found {
		t.Error("terminal update was not delivered")
	}
}

func TestSendUpdate_NilChannelAndFullChannel(t *testing.T) {
	// Nil channel must be a no-op.
	sendUpdate(nil, types.ProgressUpdate{Phase: types.PhaseFetching})

	// Full channel must not block.
	progress := make(chan types.ProgressUpdate, 1)
	progress <- types.ProgressUpdate{Phase: types.PhaseFetching}
	done := make(chan struct{})
	go func() {
		sendUpdate(progress, types.ProgressUpdate{Phase: types.PhaseFetching})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("sendUpdate blocked on a full channel")
	}
}

func TestValidateRef(t *testing.T) {
	if err := validateRef("main"); err != nil {
		t.Errorf("unexpected error for valid ref: %v", err)
	}
	if err := validateRef("v1.0"); err != nil {
		t.Errorf("unexpected error for valid tag: %v", err)
	}
	if err := validateRef(""); err == nil {
		t.Error("expected error for empty ref")
	}
	if err := validateRef("--force"); err == nil {
		t.Error("expected error for option-like ref")
	}
}

func TestIsGitRepository_GitFile(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repoDir := setupTestGitRepo(t)

	// A linked worktree has a .git *file*, not a directory.
	worktree := filepath.Join(t.TempDir(), "wt")
	gitRun(t, repoDir, "worktree", "add", worktree)

	info, err := os.Stat(filepath.Join(worktree, ".git"))
	if err != nil || info.IsDir() {
		t.Fatalf("test setup: expected .git file in worktree")
	}

	if !IsGitRepository(worktree) {
		t.Error("expected IsGitRepository to accept a worktree (.git file)")
	}
}
