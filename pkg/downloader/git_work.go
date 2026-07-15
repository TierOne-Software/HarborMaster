package downloader

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// FileChange represents a changed file and its git status.
type FileChange struct {
	Status string // "M", "A", "D", "R", "??" etc.
	Path   string
}

// CreateBranch creates a new branch at the current HEAD and checks it out.
func CreateBranch(path, branch string) error {
	if err := validateRef(branch); err != nil {
		return err
	}
	cmd := exec.Command("git", "checkout", "-b", branch, "--")
	cmd.Dir = path
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create branch %s: %w\n%s", branch, err, scrubCredentials(string(output)))
	}
	return nil
}

// CheckoutBranch checks out an existing branch.
func CheckoutBranch(path, branch string) error {
	if err := validateRef(branch); err != nil {
		return err
	}
	cmd := exec.Command("git", "checkout", branch, "--")
	cmd.Dir = path
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to checkout branch %s: %w\n%s", branch, err, scrubCredentials(string(output)))
	}
	return nil
}

// BranchExists returns true if the named local branch exists. Only
// refs/heads/ is consulted, so tags or remote refs with the same name do
// not count.
func BranchExists(path, branch string) (bool, error) {
	if err := validateRef(branch); err != nil {
		return false, err
	}
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "--", "refs/heads/"+branch)
	cmd.Dir = path
	if err := cmd.Run(); err != nil {
		// Exit code 1 means the ref does not exist; anything else is a real
		// error (e.g. not a git repository) and must be propagated.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, fmt.Errorf("failed to check branch %s: %w", branch, err)
	}
	return true, nil
}

// StageAll stages all changes (git add -A).
func StageAll(path string) error {
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = path
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to stage changes: %w\n%s", err, scrubCredentials(string(output)))
	}
	return nil
}

// Commit creates a commit with the given message and returns the new commit SHA.
func Commit(path, message string) (string, error) {
	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Dir = path
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to commit: %w\n%s", err, scrubCredentials(string(output)))
	}

	return GetHeadSHA(path)
}

// Push pushes the given branch to origin.
func Push(path, branch string) error {
	if err := validateRef(branch); err != nil {
		return err
	}
	cmd := exec.Command("git", "push", "--", "origin", branch)
	cmd.Dir = path
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to push branch %s: %w\n%s", branch, err, scrubCredentials(string(output)))
	}
	return nil
}

// PushWithUpstream pushes and sets the upstream tracking branch.
func PushWithUpstream(path, branch string) error {
	if err := validateRef(branch); err != nil {
		return err
	}
	cmd := exec.Command("git", "push", "-u", "--", "origin", branch)
	cmd.Dir = path
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to push branch %s: %w\n%s", branch, err, scrubCredentials(string(output)))
	}
	return nil
}

// RemoteBranchExists returns true if the branch exists on the origin remote.
// The match is exact: a remote branch "bar/foo" does not count as "foo".
func RemoteBranchExists(path, branch string) (bool, error) {
	if err := validateRef(branch); err != nil {
		return false, err
	}
	want := "refs/heads/" + branch
	cmd := exec.Command("git", "ls-remote", "--heads", "--", "origin", want)
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to check remote branch: %w", err)
	}

	// Output lines are "<sha>\t<ref>". ls-remote patterns match by path
	// suffix, so verify the full ref name exactly.
	for _, line := range strings.Split(string(output), "\n") {
		_, ref, found := strings.Cut(line, "\t")
		if found && strings.TrimSpace(ref) == want {
			return true, nil
		}
	}
	return false, nil
}

// GetChangedFiles returns the list of changed files with their status.
func GetChangedFiles(path string) ([]FileChange, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get changed files: %w", err)
	}

	lines := strings.Split(strings.TrimRight(string(output), "\n"), "\n")

	var changes []FileChange
	for _, line := range lines {
		change, ok := parsePorcelainLine(line)
		if !ok {
			continue
		}
		changes = append(changes, change)
	}

	return changes, nil
}

// parsePorcelainLine parses a single `git status --porcelain` (v1) line:
//
//	XY <path>
//	XY <orig-path> -> <path>   (renames and copies)
//
// Paths containing special characters are C-quoted by git. For renames the
// new path is reported.
func parsePorcelainLine(line string) (FileChange, bool) {
	if len(line) < 4 || line[2] != ' ' {
		return FileChange{}, false
	}
	status := strings.TrimSpace(line[:2])
	rest := line[3:]

	path, remainder := parsePorcelainPath(rest)
	if strings.HasPrefix(remainder, " -> ") {
		// Rename or copy: report the new path.
		path, _ = parsePorcelainPath(remainder[len(" -> "):])
	}
	if path == "" {
		return FileChange{}, false
	}

	return FileChange{Status: status, Path: path}, true
}

// parsePorcelainPath extracts one path (possibly C-quoted) from the start of
// s, returning the unquoted path and the unconsumed remainder.
func parsePorcelainPath(s string) (string, string) {
	if strings.HasPrefix(s, `"`) {
		// Find the closing quote, honoring backslash escapes.
		for i := 1; i < len(s); i++ {
			switch s[i] {
			case '\\':
				i++
			case '"':
				// Git's C-style quoting matches Go's quoted string syntax
				// (\t, \n, \", \\, \ooo octal escapes).
				if unquoted, err := strconv.Unquote(s[:i+1]); err == nil {
					return unquoted, s[i+1:]
				}
				return s, ""
			}
		}
		return s, ""
	}

	if idx := strings.Index(s, " -> "); idx >= 0 {
		return s[:idx], s[idx:]
	}
	return s, ""
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
