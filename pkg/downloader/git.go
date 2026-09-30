package downloader

import (
	"bufio"
	"bytes"
	"context"
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
		source:  opts.SourceURL,
	}
}

// Type returns the downloader type.
func (g *GitDownloader) Type() string {
	return "git"
}

// Download clones a git repository.
func (g *GitDownloader) Download(source, destination string) (string, error) {
	return g.download(source, destination, nil)
}

// DownloadWithProgress clones with progress reporting.
func (g *GitDownloader) DownloadWithProgress(source, destination string) (string, <-chan types.ProgressUpdate, error) {
	progress := make(chan types.ProgressUpdate, 10)

	go func() {
		defer close(progress)

		sha, err := g.download(source, destination, progress)
		if err != nil {
			sendTerminal(progress, types.ProgressUpdate{
				Phase: types.PhaseFailed,
				Error: err,
			})
			return
		}

		sendTerminal(progress, types.ProgressUpdate{
			Phase:   types.PhaseComplete,
			Message: sha,
		})
	}()

	return "", progress, nil
}

// download is the shared implementation for Download and DownloadWithProgress.
// progress may be nil for the blocking variant.
func (g *GitDownloader) download(source, destination string, progress chan types.ProgressUpdate) (string, error) {
	g.source = source

	sendUpdate(progress, types.ProgressUpdate{
		Phase:   types.PhaseConnecting,
		Message: "Connecting to remote...",
	})

	args := g.cloneArgs(source, destination, progress != nil)

	sendUpdate(progress, types.ProgressUpdate{
		Phase:   types.PhaseFetching,
		Message: "Cloning repository...",
	})

	if err := g.runGitStreaming("", progress, args...); err != nil {
		return "", fmt.Errorf("failed to clone: %w", err)
	}

	// A pinned commit may not be reachable from any branch or tag tip (e.g.
	// after a force-push); fall back to fetching it directly, exactly as the
	// update path does.
	if err := g.ensureCommitAvailable(destination, progress); err != nil {
		return "", err
	}

	if g.options.GetEffectiveRef() != "" {
		sendUpdate(progress, types.ProgressUpdate{
			Phase:   types.PhaseCheckout,
			Message: "Checking out ref...",
		})
	}

	if err := g.checkoutRef(destination, false); err != nil {
		return "", err
	}

	// The clone initialized submodules for the remote tip; after checking
	// out a pinned commit (locked sync), the gitlinks may point elsewhere —
	// resynchronize so the tree actually matches the pin.
	if err := g.updateSubmodules(destination, progress); err != nil {
		return "", err
	}

	return GetHeadSHA(destination)
}

// cloneArgs builds the argument list for git clone.
func (g *GitDownloader) cloneArgs(source, destination string, withProgress bool) []string {
	args := []string{"clone"}

	if withProgress {
		args = append(args, "--progress")
	}

	// A pinned commit may be arbitrarily deep in history (or on a non-default
	// branch), and a shallow clone only contains the branch tip. Do a full
	// clone whenever a commit is pinned so the checkout below can succeed.
	if g.options.Commit == "" && g.options.Shallow && g.options.Depth > 0 {
		// --depth implies --single-branch, which would prevent switching to
		// another branch on a later update; explicitly disable it.
		args = append(args, "--depth", fmt.Sprintf("%d", g.options.Depth), "--no-single-branch")
	}

	if g.options.Commit == "" {
		if g.options.Tag != "" {
			args = append(args, "--branch", g.options.Tag)
		} else if g.options.Branch != "" {
			args = append(args, "--branch", g.options.Branch)
		}
	}

	if g.options.Submodules {
		args = append(args, "--recurse-submodules")
	}

	args = append(args, "--", source, destination)
	return args
}

// Update fetches and checks out the latest changes.
func (g *GitDownloader) Update(destination string) (string, error) {
	return g.update(destination, nil)
}

// UpdateWithProgress updates with progress reporting.
func (g *GitDownloader) UpdateWithProgress(destination string) (string, <-chan types.ProgressUpdate, error) {
	progress := make(chan types.ProgressUpdate, 10)

	go func() {
		defer close(progress)

		sha, err := g.update(destination, progress)
		if err != nil {
			sendTerminal(progress, types.ProgressUpdate{
				Phase: types.PhaseFailed,
				Error: err,
			})
			return
		}

		sendTerminal(progress, types.ProgressUpdate{
			Phase:   types.PhaseComplete,
			Message: sha,
		})
	}()

	return "", progress, nil
}

// update is the shared implementation for Update and UpdateWithProgress.
// progress may be nil for the blocking variant.
func (g *GitDownloader) update(destination string, progress chan types.ProgressUpdate) (string, error) {
	sendUpdate(progress, types.ProgressUpdate{
		Phase:   types.PhaseFetching,
		Message: "Fetching updates...",
	})

	// Older clones may have a single-branch fetch refspec that hides other
	// branches (and pinned commits on them); widen it before fetching.
	g.widenFetchRefspec(destination)

	fetchArgs := []string{"fetch"}
	if progress != nil {
		fetchArgs = append(fetchArgs, "--progress")
	}
	fetchArgs = append(fetchArgs, "--force", "--tags", "origin")

	if err := g.runGitStreaming(destination, progress, fetchArgs...); err != nil {
		return "", fmt.Errorf("failed to fetch: %w", err)
	}

	if err := g.ensureCommitAvailable(destination, progress); err != nil {
		return "", err
	}

	sendUpdate(progress, types.ProgressUpdate{
		Phase:   types.PhaseCheckout,
		Message: "Checking out...",
	})

	if err := g.checkoutRef(destination, true); err != nil {
		return "", err
	}

	if err := g.updateSubmodules(destination, progress); err != nil {
		return "", err
	}

	return GetHeadSHA(destination)
}

// updateSubmodules synchronizes submodule checkouts to the gitlinks recorded
// in the newly checked-out commit. Without this, an update that moves HEAD
// across a submodule bump leaves the submodule working trees at their old
// commits — silently stale content, and a phantom "dirty" in the parent's
// porcelain status.
func (g *GitDownloader) updateSubmodules(destination string, progress chan types.ProgressUpdate) error {
	if _, err := os.Stat(filepath.Join(destination, ".gitmodules")); err != nil {
		if os.IsNotExist(err) {
			return nil // no submodules in this commit
		}
		// Permission, symlink-loop, or I/O errors must not silently skip
		// synchronization and report success with stale submodules.
		return fmt.Errorf("failed to check for submodules: %w", err)
	}

	if !g.options.Submodules {
		sendUpdate(progress, types.ProgressUpdate{
			Phase:   types.PhaseCheckout,
			Message: "note: repository has submodules but submodules are disabled; skipping submodule update",
		})
		return nil
	}

	sendUpdate(progress, types.ProgressUpdate{
		Phase:   types.PhaseCheckout,
		Message: "Updating submodules...",
	})

	// --force matches the parent's checkout --force semantics: sync is
	// authoritative, a dirty submodule is not a reason to stay stale.
	if err := g.runGitStreaming(destination, progress,
		"submodule", "update", "--init", "--recursive", "--force"); err != nil {
		return fmt.Errorf("failed to update submodules: %w", err)
	}
	return nil
}

// widenFetchRefspec ensures the origin fetch refspec covers all branches.
// Single-branch clones record a refspec limited to one branch, which would
// prevent later branch switches or pinned-commit fetches. Best effort.
func (g *GitDownloader) widenFetchRefspec(destination string) {
	out, err := g.runGit(destination, "config", "--get-all", "remote.origin.fetch")
	if err != nil {
		return
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.Contains(line, "refs/heads/*") {
			return
		}
	}
	_, _ = g.runGit(destination, "config", "--replace-all", "remote.origin.fetch",
		"+refs/heads/*:refs/remotes/origin/*")
}

// ensureCommitAvailable makes sure a pinned commit is present locally,
// unshallowing or fetching the commit directly when necessary.
func (g *GitDownloader) ensureCommitAvailable(destination string, progress chan types.ProgressUpdate) error {
	sha := g.options.Commit
	if sha == "" {
		return nil
	}
	if err := validateRef(sha); err != nil {
		return err
	}
	if g.hasCommit(destination, sha) {
		return nil
	}

	// The commit may be beyond the shallow boundary.
	if g.isShallow(destination) {
		sendUpdate(progress, types.ProgressUpdate{
			Phase:   types.PhaseFetching,
			Message: "Fetching full history...",
		})
		if _, err := g.runGit(destination, "fetch", "--unshallow", "origin"); err == nil {
			if g.hasCommit(destination, sha) {
				return nil
			}
		}
	}

	// Last resort: ask the server for the commit directly (requires the
	// server to allow fetching by SHA, which GitHub and GitLab do).
	if _, err := g.runGit(destination, "fetch", "origin", sha); err == nil {
		if g.hasCommit(destination, sha) {
			return nil
		}
	}

	return fmt.Errorf("commit %s not found in repository after fetching", sha)
}

// hasCommit reports whether the given commit exists in the repository.
func (g *GitDownloader) hasCommit(destination, sha string) bool {
	_, err := g.runGit(destination, "cat-file", "-e", sha+"^{commit}")
	return err == nil
}

// isShallow reports whether the repository is a shallow clone.
func (g *GitDownloader) isShallow(destination string) bool {
	out, err := g.runGit(destination, "rev-parse", "--is-shallow-repository")
	return err == nil && strings.TrimSpace(out) == "true"
}

// GetCurrentRef returns the current HEAD commit SHA.
func (g *GitDownloader) GetCurrentRef(destination string) (string, error) {
	return GetHeadSHA(destination)
}

// checkoutRef checks out the configured ref (commit > tag > branch). When no
// ref is configured and forUpdate is true, it advances to the remote default
// branch so an update actually moves HEAD forward.
func (g *GitDownloader) checkoutRef(destination string, forUpdate bool) error {
	var ref string

	switch {
	case g.options.Commit != "":
		ref = g.options.Commit
	case g.options.Tag != "":
		ref = g.options.Tag
	case g.options.Branch != "":
		ref = "origin/" + g.options.Branch
	default:
		if !forUpdate {
			// Fresh clone already has the default branch checked out.
			return nil
		}
		ref = g.remoteDefaultRef(destination)
		if ref == "" {
			return nil
		}
	}

	if err := validateRef(ref); err != nil {
		return err
	}

	if _, err := g.runGit(destination, "checkout", "--force", "--detach", ref, "--"); err != nil {
		return fmt.Errorf("failed to checkout %s: %w", ref, err)
	}

	return nil
}

// remoteDefaultRef returns the remote-tracking ref to sync to when no ref is
// configured (e.g. "origin/main"), or "" if it cannot be determined.
func (g *GitDownloader) remoteDefaultRef(destination string) string {
	// Prefer the remote default branch recorded at clone time.
	if out, err := g.runGit(destination, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"); err == nil {
		if ref := strings.TrimSpace(out); strings.HasPrefix(ref, "refs/remotes/") {
			return strings.TrimPrefix(ref, "refs/remotes/")
		}
	}
	// Fall back to the currently checked out branch.
	if branch, err := GetCurrentBranch(destination); err == nil && branch != "" {
		return "origin/" + branch
	}
	return ""
}

// newContext returns a context honoring Options.Timeout.
func (g *GitDownloader) newContext() (context.Context, context.CancelFunc) {
	if g.options.Timeout > 0 {
		return context.WithTimeout(context.Background(), g.options.Timeout)
	}
	return context.WithCancel(context.Background())
}

// runGit runs a git command in dir, honoring Options.Timeout. Returned errors
// have embedded git output scrubbed of URL credentials.
func (g *GitDownloader) runGit(dir string, args ...string) (string, error) {
	ctx, cancel := g.newContext()
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("git %s timed out after %s", args[0], g.options.Timeout)
		}
		return "", fmt.Errorf("git %s failed: %w\n%s", args[0], err, scrubCredentials(string(output)))
	}
	return string(output), nil
}

// runGitStreaming runs a git command, streaming progress lines parsed from
// stderr to the given channel (which may be nil). Errors embed a scrubbed
// tail of the git output.
func (g *GitDownloader) runGitStreaming(dir string, progress chan types.ProgressUpdate, args ...string) error {
	ctx, cancel := g.newContext()
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}

	// Git writes both progress and error details to stderr.
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("git %s timed out after %s", args[0], g.options.Timeout)
		}
		return fmt.Errorf("failed to start git: %w", err)
	}

	var tail []string
	scanner := bufio.NewScanner(stderr)
	scanner.Split(scanGitProgress)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		tail = append(tail, line)
		if len(tail) > 5 {
			tail = tail[1:]
		}

		update := types.ProgressUpdate{
			Phase:   types.PhaseFetching,
			Message: scrubCredentials(line),
		}
		if pct := extractPercentage(line); pct >= 0 {
			update.BytesDone = int64(pct)
			update.BytesTotal = 100
		}
		sendUpdate(progress, update)
	}
	scanErr := scanner.Err()

	if err := cmd.Wait(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("git %s timed out after %s", args[0], g.options.Timeout)
		}
		return fmt.Errorf("git %s failed: %w\n%s", args[0], err, scrubCredentials(strings.Join(tail, "\n")))
	}
	if scanErr != nil {
		return fmt.Errorf("failed to read git output: %w", scanErr)
	}

	return nil
}

// sendUpdate delivers a non-terminal progress update without blocking.
// It is a no-op when progress is nil; updates are dropped if the channel
// buffer is full.
func sendUpdate(progress chan<- types.ProgressUpdate, update types.ProgressUpdate) {
	if progress == nil {
		return
	}
	select {
	case progress <- update:
	default:
	}
}

// sendTerminal delivers a terminal (complete/failed) update. Unlike
// sendUpdate it guarantees delivery, but never blocks indefinitely: if the
// buffer is full it discards stale buffered updates to make room, so the
// producing goroutine cannot leak even when the consumer stops reading.
func sendTerminal(progress chan types.ProgressUpdate, update types.ProgressUpdate) {
	if progress == nil {
		return
	}
	for {
		select {
		case progress <- update:
			return
		case <-progress:
			// Buffer was full: dropped the oldest buffered update; retry.
		}
	}
}

// validateRef rejects ref names that could be interpreted as command-line
// options by git.
func validateRef(ref string) error {
	if ref == "" || strings.HasPrefix(ref, "-") {
		return fmt.Errorf("invalid git ref: %q", ref)
	}
	return nil
}

// credentialRegex matches userinfo (user or user:password) embedded in URLs.
var credentialRegex = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)[^/@\s]+@`)

// scrubCredentials removes credentials embedded in URLs (user:token@host)
// from a string so they never leak into error messages or progress output.
func scrubCredentials(s string) string {
	return credentialRegex.ReplaceAllString(s, "$1***@")
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

// IsGitRepository returns true if the path is a git repository. Both a .git
// directory (normal clone) and a .git file (worktree or submodule) count.
func IsGitRepository(path string) bool {
	gitDir := filepath.Join(path, ".git")
	_, err := os.Stat(gitDir)
	return err == nil
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

// ResolveRemoteRef returns the commit SHA that ref points at on the remote,
// without needing a local clone. ref should be a fully-qualified ref such as
// "refs/heads/main". The URL may carry credentials; they are scrubbed from
// any error message.
func ResolveRemoteRef(url, ref string) (string, error) {
	if err := validateRef(ref); err != nil {
		return "", err
	}
	cmd := exec.Command("git", "ls-remote", url, ref)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to resolve %s on remote: %w", ref, err)
	}
	// Output is "<sha>	<ref>" per matching line; we queried one exact ref.
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == ref {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("ref %s not found on remote %s", ref, scrubCredentials(url))
}

// CheckSubmodules returns the paths of submodules whose checkout does not
// match the superproject's recorded gitlink — uninitialized ('-' prefix in
// git submodule status), checked out at a different commit ('+'), or in a
// merge-conflicted state ('U'). An up-to-date tree yields an empty slice.
func CheckSubmodules(path string) ([]string, error) {
	cmd := exec.Command("git", "submodule", "status", "--recursive")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get submodule status: %w", err)
	}
	return parseSubmoduleStatus(string(output)), nil
}

// parseSubmoduleStatus extracts the paths of non-clean submodules from
// 'git submodule status' output.
func parseSubmoduleStatus(output string) []string {
	var mismatched []string
	for _, line := range strings.Split(strings.TrimRight(output, "\n"), "\n") {
		if line == "" {
			continue
		}
		// Format: <status><sha> <path> [(describe)]; ' ' means clean.
		switch line[0] {
		case '+', '-', 'U':
			fields := strings.Fields(line[1:])
			if len(fields) >= 2 {
				mismatched = append(mismatched, fields[1])
			}
		}
	}
	return mismatched
}

// RemoteContains reports whether any remote-tracking ref in the repository
// at path contains the given commit — i.e. whether the commit has been
// published to a remote this clone knows about.
func RemoteContains(path, commit string) (bool, error) {
	if err := validateRef(commit); err != nil {
		return false, err
	}
	cmd := exec.Command("git", "branch", "-r", "--contains", commit)
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to check remote containment: %w", err)
	}
	return strings.TrimSpace(string(output)) != "", nil
}

// Exists returns true if the destination exists.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
