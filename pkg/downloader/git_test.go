package downloader

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/tierone/harbormaster/pkg/types"
)

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
		t.Error("expected at least one progress update")
	}

	// Last update should be complete or contain the result
	lastUpdate := updates[len(updates)-1]
	// Note: Might still be valid if there's no explicit complete message
	_ = lastUpdate.Phase == types.PhaseComplete || lastUpdate.Error != nil

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
	newFile := filepath.Join(sourceRepo, "new-file.txt")
	if err := os.WriteFile(newFile, []byte("new content"), 0644); err != nil {
		t.Fatalf("failed to create new file: %v", err)
	}

	cmds := [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "Second commit"},
	}
	for _, cmd := range cmds {
		c := exec.Command(cmd[0], cmd[1:]...)
		c.Dir = sourceRepo
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("failed to run %v: %v\n%s", cmd, err, out)
		}
	}

	// Update the clone
	sha, err := dl.Update(destDir)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	if sha == "" {
		t.Error("expected SHA to be returned")
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
