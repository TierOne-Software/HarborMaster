package downloader

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tierone/harbormaster/pkg/types"
)

// GitDownloader implements Downloader for Git repositories using native git command.
type GitDownloader struct {
	options Options
	source  string
}

// NewGitDownloader creates a new GitDownloader with the given options.
func NewGitDownloader(opts Options) *GitDownloader {
	return &GitDownloader{
		options: opts,
	}
}

// Type returns the downloader type.
func (g *GitDownloader) Type() string {
	return "git"
}

// Download clones a git repository.
func (g *GitDownloader) Download(source, destination string) (string, error) {
	g.source = source

	// Build clone command
	args := []string{"clone"}

	if g.options.Shallow && g.options.Depth > 0 {
		args = append(args, "--depth", fmt.Sprintf("%d", g.options.Depth))
	}

	if g.options.Branch != "" && g.options.Tag == "" && g.options.Commit == "" {
		args = append(args, "--branch", g.options.Branch)
		args = append(args, "--single-branch")
	}

	if g.options.Submodules {
		args = append(args, "--recurse-submodules")
	}

	args = append(args, source, destination)

	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to clone: %w\n%s", err, string(output))
	}

	// Checkout specific ref if needed
	if err := g.checkoutRef(destination); err != nil {
		return "", err
	}

	return g.getHeadSHA(destination)
}

// DownloadWithProgress clones with progress reporting.
func (g *GitDownloader) DownloadWithProgress(source, destination string) (string, <-chan types.ProgressUpdate, error) {
	g.source = source
	progress := make(chan types.ProgressUpdate, 10)

	go func() {
		defer close(progress)

		progress <- types.ProgressUpdate{
			Phase:   types.PhaseConnecting,
			Message: "Connecting to remote...",
		}

		// Build clone command
		args := []string{"clone", "--progress"}

		if g.options.Shallow && g.options.Depth > 0 {
			args = append(args, "--depth", fmt.Sprintf("%d", g.options.Depth))
		}

		if g.options.Branch != "" && g.options.Tag == "" && g.options.Commit == "" {
			args = append(args, "--branch", g.options.Branch)
			args = append(args, "--single-branch")
		}

		if g.options.Submodules {
			args = append(args, "--recurse-submodules")
		}

		args = append(args, source, destination)

		progress <- types.ProgressUpdate{
			Phase:   types.PhaseFetching,
			Message: "Cloning repository...",
		}

		cmd := exec.Command("git", args...)

		// Git outputs progress to stderr
		stderr, err := cmd.StderrPipe()
		if err != nil {
			progress <- types.ProgressUpdate{
				Phase: types.PhaseFailed,
				Error: fmt.Errorf("failed to create pipe: %w", err),
			}
			return
		}

		if err := cmd.Start(); err != nil {
			progress <- types.ProgressUpdate{
				Phase: types.PhaseFailed,
				Error: fmt.Errorf("failed to start git: %w", err),
			}
			return
		}

		// Parse progress from stderr
		scanner := bufio.NewScanner(stderr)
		scanner.Split(scanGitProgress)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			update := types.ProgressUpdate{
				Phase:   types.PhaseFetching,
				Message: line,
			}

			if pct := extractPercentage(line); pct >= 0 {
				update.BytesDone = int64(pct)
				update.BytesTotal = 100
			}

			select {
			case progress <- update:
			default:
			}
		}

		if err := cmd.Wait(); err != nil {
			progress <- types.ProgressUpdate{
				Phase: types.PhaseFailed,
				Error: fmt.Errorf("clone failed: %w", err),
			}
			return
		}

		// Checkout specific ref if needed
		if g.options.Commit != "" || g.options.Tag != "" {
			progress <- types.ProgressUpdate{
				Phase:   types.PhaseCheckout,
				Message: "Checking out ref...",
			}
			if err := g.checkoutRef(destination); err != nil {
				progress <- types.ProgressUpdate{
					Phase: types.PhaseFailed,
					Error: err,
				}
				return
			}
		}

		sha, err := g.getHeadSHA(destination)
		if err != nil {
			progress <- types.ProgressUpdate{
				Phase: types.PhaseFailed,
				Error: err,
			}
			return
		}

		progress <- types.ProgressUpdate{
			Phase:   types.PhaseComplete,
			Message: sha,
		}
	}()

	return "", progress, nil
}

// Update fetches and checks out the latest changes.
func (g *GitDownloader) Update(destination string) (string, error) {
	// Fetch from origin
	cmd := exec.Command("git", "fetch", "--all", "--force")
	cmd.Dir = destination
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to fetch: %w\n%s", err, string(output))
	}

	// Checkout the requested ref
	if err := g.checkoutRef(destination); err != nil {
		return "", err
	}

	return g.getHeadSHA(destination)
}

// UpdateWithProgress updates with progress reporting.
func (g *GitDownloader) UpdateWithProgress(destination string) (string, <-chan types.ProgressUpdate, error) {
	progress := make(chan types.ProgressUpdate, 10)

	go func() {
		defer close(progress)

		progress <- types.ProgressUpdate{
			Phase:   types.PhaseFetching,
			Message: "Fetching updates...",
		}

		cmd := exec.Command("git", "fetch", "--all", "--force", "--progress")
		cmd.Dir = destination

		stderr, err := cmd.StderrPipe()
		if err != nil {
			progress <- types.ProgressUpdate{
				Phase: types.PhaseFailed,
				Error: fmt.Errorf("failed to create pipe: %w", err),
			}
			return
		}

		if err := cmd.Start(); err != nil {
			progress <- types.ProgressUpdate{
				Phase: types.PhaseFailed,
				Error: fmt.Errorf("failed to start git: %w", err),
			}
			return
		}

		scanner := bufio.NewScanner(stderr)
		scanner.Split(scanGitProgress)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			update := types.ProgressUpdate{
				Phase:   types.PhaseFetching,
				Message: line,
			}

			if pct := extractPercentage(line); pct >= 0 {
				update.BytesDone = int64(pct)
				update.BytesTotal = 100
			}

			select {
			case progress <- update:
			default:
			}
		}

		if err := cmd.Wait(); err != nil {
			progress <- types.ProgressUpdate{
				Phase: types.PhaseFailed,
				Error: fmt.Errorf("fetch failed: %w", err),
			}
			return
		}

		progress <- types.ProgressUpdate{
			Phase:   types.PhaseCheckout,
			Message: "Checking out...",
		}

		if err := g.checkoutRef(destination); err != nil {
			progress <- types.ProgressUpdate{
				Phase: types.PhaseFailed,
				Error: err,
			}
			return
		}

		sha, err := g.getHeadSHA(destination)
		if err != nil {
			progress <- types.ProgressUpdate{
				Phase: types.PhaseFailed,
				Error: err,
			}
			return
		}

		progress <- types.ProgressUpdate{
			Phase:   types.PhaseComplete,
			Message: sha,
		}
	}()

	return "", progress, nil
}

// GetCurrentRef returns the current HEAD commit SHA.
func (g *GitDownloader) GetCurrentRef(destination string) (string, error) {
	return g.getHeadSHA(destination)
}

func (g *GitDownloader) checkoutRef(destination string) error {
	var ref string

	if g.options.Commit != "" {
		ref = g.options.Commit
	} else if g.options.Tag != "" {
		ref = g.options.Tag
	} else if g.options.Branch != "" {
		ref = "origin/" + g.options.Branch
	} else {
		return nil
	}

	cmd := exec.Command("git", "checkout", "--force", ref)
	cmd.Dir = destination
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to checkout %s: %w\n%s", ref, err, string(output))
	}

	return nil
}

func (g *GitDownloader) getHeadSHA(destination string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = destination
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD SHA: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// scanGitProgress is a split function for bufio.Scanner that handles git's progress output.
// Git uses \r to update progress lines, so we split on \r and \n.
func scanGitProgress(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	// Look for \r or \n
	if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
		return i + 1, data[0:i], nil
	}

	if atEOF {
		return len(data), data, nil
	}

	return 0, nil, nil
}

// extractPercentage tries to extract a percentage from git output.
var percentRegex = regexp.MustCompile(`(\d+)%`)

func extractPercentage(s string) int {
	matches := percentRegex.FindStringSubmatch(s)
	if len(matches) >= 2 {
		var pct int
		_, _ = fmt.Sscanf(matches[1], "%d", &pct)
		return pct
	}
	return -1
}

// IsGitRepository returns true if the path is a git repository.
func IsGitRepository(path string) bool {
	gitDir := filepath.Join(path, ".git")
	info, err := os.Stat(gitDir)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// GetRemoteURL returns the origin remote URL of a git repository.
func GetRemoteURL(path string) (string, error) {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get remote URL: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// IsDirty returns true if the repository has uncommitted changes.
func IsDirty(path string) (bool, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to get status: %w", err)
	}
	return len(strings.TrimSpace(string(output))) > 0, nil
}

// GetCurrentBranch returns the current branch name.
func GetCurrentBranch(path string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get branch: %w", err)
	}
	branch := strings.TrimSpace(string(output))
	if branch == "HEAD" {
		// Detached HEAD
		return "", nil
	}
	return branch, nil
}

// Exists returns true if the destination exists.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
