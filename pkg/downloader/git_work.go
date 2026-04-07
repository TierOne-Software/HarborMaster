package downloader

import (
	"fmt"
	"os/exec"
	"strings"
)

// FileChange represents a changed file and its git status.
type FileChange struct {
	Status string // "M", "A", "D", "R", "??" etc.
	Path   string
}

// CreateBranch creates a new branch at the current HEAD and checks it out.
func CreateBranch(path, branch string) error {
	cmd := exec.Command("git", "checkout", "-b", branch)
	cmd.Dir = path
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create branch %s: %w\n%s", branch, err, string(output))
	}
	return nil
}

// CheckoutBranch checks out an existing branch.
func CheckoutBranch(path, branch string) error {
	cmd := exec.Command("git", "checkout", branch)
	cmd.Dir = path
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to checkout branch %s: %w\n%s", branch, err, string(output))
	}
	return nil
}

// BranchExists returns true if the named branch exists locally.
func BranchExists(path, branch string) (bool, error) {
	cmd := exec.Command("git", "rev-parse", "--verify", branch)
	cmd.Dir = path
	if err := cmd.Run(); err != nil {
		return false, nil
	}
	return true, nil
}

// StageAll stages all changes (git add -A).
func StageAll(path string) error {
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = path
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to stage changes: %w\n%s", err, string(output))
	}
	return nil
}

// Commit creates a commit with the given message and returns the new commit SHA.
func Commit(path, message string) (string, error) {
	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Dir = path
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to commit: %w\n%s", err, string(output))
	}

	// Get the new commit SHA
	shaCmd := exec.Command("git", "rev-parse", "HEAD")
	shaCmd.Dir = path
	output, err := shaCmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get commit SHA: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// Push pushes the given branch to origin.
func Push(path, branch string) error {
	cmd := exec.Command("git", "push", "origin", branch)
	cmd.Dir = path
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to push branch %s: %w\n%s", branch, err, string(output))
	}
	return nil
}

// PushWithUpstream pushes and sets the upstream tracking branch.
func PushWithUpstream(path, branch string) error {
	cmd := exec.Command("git", "push", "-u", "origin", branch)
	cmd.Dir = path
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to push branch %s: %w\n%s", branch, err, string(output))
	}
	return nil
}

// RemoteBranchExists returns true if the branch exists on the origin remote.
func RemoteBranchExists(path, branch string) (bool, error) {
	cmd := exec.Command("git", "ls-remote", "--heads", "origin", branch)
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to check remote branch: %w", err)
	}
	return len(strings.TrimSpace(string(output))) > 0, nil
}

// GetChangedFiles returns the list of changed files with their status.
func GetChangedFiles(path string) ([]FileChange, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get changed files: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil, nil
	}

	changes := make([]FileChange, 0, len(lines))
	for _, line := range lines {
		if len(line) < 4 {
			continue
		}
		changes = append(changes, FileChange{
			Status: strings.TrimSpace(line[:2]),
			Path:   strings.TrimSpace(line[3:]),
		})
	}

	return changes, nil
}

// GetHeadSHA returns the current HEAD commit SHA.
func GetHeadSHA(path string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD SHA: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}
