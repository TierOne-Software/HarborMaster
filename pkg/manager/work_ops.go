package manager

import (
	"fmt"
	"time"

	"github.com/tierone/harbormaster/pkg/config"
	"github.com/tierone/harbormaster/pkg/downloader"
	"github.com/tierone/harbormaster/pkg/work"
)

// WorkRepoStatus represents the status of a repository in a work session.
type WorkRepoStatus struct {
	Name           string
	Path           string
	Branch         string
	OriginalBranch string
	IsDirty        bool
	IsOnBranch     bool // true if currently on the work session branch
	ChangedFiles   []downloader.FileChange
	Error          error
}

// WorkCommitResult represents the result of committing in a repository.
type WorkCommitResult struct {
	RepoName string
	SHA      string
	Success  bool
	Error    error
}

// WorkPushResult represents the result of pushing a repository.
type WorkPushResult struct {
	RepoName string
	Success  bool
	Error    error
}

// WorkPRResult represents the result of creating a PR for a repository.
type WorkPRResult struct {
	RepoName string
	URL      string
	Success  bool
	Error    error
}

// WorkStart creates a new work session, creating a branch across the selected repositories.
func (m *RepositoryManager) WorkStart(name, branch string, filter Filter) (*work.WorkSession, error) {
	// Get repositories matching the filter
	repos, err := m.getRepositories(filter)
	if err != nil {
		return nil, err
	}

	if len(repos) == 0 {
		return nil, fmt.Errorf("no repositories matched the filter")
	}

	ws := work.New(name, branch)
	var skipped []string
	var dirty []string

	for _, repo := range repos {
		// Skip non-git repos
		if repo.Type != config.RepoTypeGit {
			skipped = append(skipped, repo.Name)
			continue
		}

		repoPath := m.getRepoPath(&repo)

		// Check if repo exists on disk
		if !downloader.Exists(repoPath) {
			return nil, fmt.Errorf("repository %s not found at %s (run 'hm sync' first)", repo.Name, repoPath)
		}

		// Check if repo is dirty
		isDirty, err := downloader.IsDirty(repoPath)
		if err != nil {
			return nil, fmt.Errorf("failed to check status of %s: %w", repo.Name, err)
		}
		if isDirty {
			dirty = append(dirty, repo.Name)
			continue
		}

		// Get current branch and SHA
		currentBranch, err := downloader.GetCurrentBranch(repoPath)
		if err != nil {
			return nil, fmt.Errorf("failed to get current branch of %s: %w", repo.Name, err)
		}

		currentSHA, err := downloader.GetHeadSHA(repoPath)
		if err != nil {
			return nil, fmt.Errorf("failed to get HEAD SHA of %s: %w", repo.Name, err)
		}

		// If detached HEAD, use SHA as the original ref
		originalBranch := currentBranch
		if originalBranch == "" {
			originalBranch = currentSHA
		}

		// Create or checkout branch
		exists, err := downloader.BranchExists(repoPath, branch)
		if err != nil {
			return nil, fmt.Errorf("failed to check branch in %s: %w", repo.Name, err)
		}

		if exists {
			if err := downloader.CheckoutBranch(repoPath, branch); err != nil {
				return nil, fmt.Errorf("failed to checkout branch in %s: %w", repo.Name, err)
			}
		} else {
			if err := downloader.CreateBranch(repoPath, branch); err != nil {
				return nil, fmt.Errorf("failed to create branch in %s: %w", repo.Name, err)
			}
		}

		// Add repo to session
		_ = ws.AddRepo(work.WorkRepo{
			Name:           repo.Name,
			OriginalBranch: originalBranch,
			OriginalSHA:    currentSHA,
			AddedAt:        time.Now(),
		})
	}

	if len(dirty) > 0 {
		return nil, fmt.Errorf("the following repositories have uncommitted changes: %v\nPlease commit or stash changes before starting a work session", dirty)
	}

	if len(ws.Repos) == 0 {
		if len(skipped) > 0 {
			return nil, fmt.Errorf("no git repositories matched the filter (skipped HTTP repos: %v)", skipped)
		}
		return nil, fmt.Errorf("no repositories matched the filter")
	}

	return ws, nil
}

// WorkAdd adds a single repository to an existing work session.
func (m *RepositoryManager) WorkAdd(ws *work.WorkSession, repoName string) error {
	repo, ok := m.config.GetRepository(repoName)
	if !ok {
		return fmt.Errorf("repository not found: %s", repoName)
	}

	if repo.Type != config.RepoTypeGit {
		return fmt.Errorf("only git repositories can be added to work sessions (got %s)", repo.Type)
	}

	if ws.HasRepo(repoName) {
		return fmt.Errorf("repository %s is already in the work session", repoName)
	}

	repoPath := m.getRepoPath(repo)

	if !downloader.Exists(repoPath) {
		return fmt.Errorf("repository %s not found at %s (run 'hm sync' first)", repoName, repoPath)
	}

	isDirty, err := downloader.IsDirty(repoPath)
	if err != nil {
		return fmt.Errorf("failed to check status of %s: %w", repoName, err)
	}
	if isDirty {
		return fmt.Errorf("repository %s has uncommitted changes; commit or stash first", repoName)
	}

	currentBranch, err := downloader.GetCurrentBranch(repoPath)
	if err != nil {
		return fmt.Errorf("failed to get current branch of %s: %w", repoName, err)
	}

	currentSHA, err := downloader.GetHeadSHA(repoPath)
	if err != nil {
		return fmt.Errorf("failed to get HEAD SHA of %s: %w", repoName, err)
	}

	originalBranch := currentBranch
	if originalBranch == "" {
		originalBranch = currentSHA
	}

	// Create or checkout the work session branch
	exists, err := downloader.BranchExists(repoPath, ws.Branch)
	if err != nil {
		return fmt.Errorf("failed to check branch in %s: %w", repoName, err)
	}

	if exists {
		if err := downloader.CheckoutBranch(repoPath, ws.Branch); err != nil {
			return fmt.Errorf("failed to checkout branch in %s: %w", repoName, err)
		}
	} else {
		if err := downloader.CreateBranch(repoPath, ws.Branch); err != nil {
			return fmt.Errorf("failed to create branch in %s: %w", repoName, err)
		}
	}

	return ws.AddRepo(work.WorkRepo{
		Name:           repoName,
		OriginalBranch: originalBranch,
		OriginalSHA:    currentSHA,
		AddedAt:        time.Now(),
	})
}

// WorkRemove removes a repository from the work session, restoring its original branch.
func (m *RepositoryManager) WorkRemove(ws *work.WorkSession, repoName string) error {
	wr, ok := ws.GetRepo(repoName)
	if !ok {
		return fmt.Errorf("repository %s is not in the work session", repoName)
	}

	repo, ok := m.config.GetRepository(repoName)
	if !ok {
		return fmt.Errorf("repository not found in config: %s", repoName)
	}

	repoPath := m.getRepoPath(repo)

	// Checkout original branch
	if err := downloader.CheckoutBranch(repoPath, wr.OriginalBranch); err != nil {
		return fmt.Errorf("failed to restore original branch for %s: %w", repoName, err)
	}

	return ws.RemoveRepo(repoName)
}

// WorkStatus returns the status of all repositories in the work session.
func (m *RepositoryManager) WorkStatus(ws *work.WorkSession) ([]WorkRepoStatus, error) {
	statuses := make([]WorkRepoStatus, 0, len(ws.Repos))

	for _, wr := range ws.Repos {
		status := WorkRepoStatus{
			Name:           wr.Name,
			OriginalBranch: wr.OriginalBranch,
		}

		repo, ok := m.config.GetRepository(wr.Name)
		if !ok {
			status.Error = fmt.Errorf("repository not found in config")
			statuses = append(statuses, status)
			continue
		}

		repoPath := m.getRepoPath(repo)
		status.Path = repoPath

		if !downloader.Exists(repoPath) {
			status.Error = fmt.Errorf("repository not found on disk")
			statuses = append(statuses, status)
			continue
		}

		branch, err := downloader.GetCurrentBranch(repoPath)
		if err != nil {
			status.Error = err
			statuses = append(statuses, status)
			continue
		}
		status.Branch = branch
		status.IsOnBranch = branch == ws.Branch

		dirty, err := downloader.IsDirty(repoPath)
		if err != nil {
			status.Error = err
			statuses = append(statuses, status)
			continue
		}
		status.IsDirty = dirty

		if dirty {
			changes, err := downloader.GetChangedFiles(repoPath)
			if err != nil {
				status.Error = err
				statuses = append(statuses, status)
				continue
			}
			status.ChangedFiles = changes
		}

		statuses = append(statuses, status)
	}

	return statuses, nil
}

// WorkCommit commits changes across selected work session repositories.
// If repoNames is empty, commits across all repos in the session.
func (m *RepositoryManager) WorkCommit(ws *work.WorkSession, message string, repoNames []string) ([]WorkCommitResult, error) {
	targets := repoNames
	if len(targets) == 0 {
		targets = ws.RepoNames()
	}

	results := make([]WorkCommitResult, 0, len(targets))

	for _, name := range targets {
		result := WorkCommitResult{RepoName: name}

		if !ws.HasRepo(name) {
			result.Error = fmt.Errorf("repository %s is not in the work session", name)
			results = append(results, result)
			continue
		}

		repo, ok := m.config.GetRepository(name)
		if !ok {
			result.Error = fmt.Errorf("repository not found in config")
			results = append(results, result)
			continue
		}

		repoPath := m.getRepoPath(repo)

		// Check if there are changes to commit
		isDirty, err := downloader.IsDirty(repoPath)
		if err != nil {
			result.Error = err
			results = append(results, result)
			continue
		}

		if !isDirty {
			result.Success = true // nothing to commit is not an error
			results = append(results, result)
			continue
		}

		// Stage all changes
		if err := downloader.StageAll(repoPath); err != nil {
			result.Error = err
			results = append(results, result)
			continue
		}

		// Commit
		sha, err := downloader.Commit(repoPath, message)
		if err != nil {
			result.Error = err
			results = append(results, result)
			continue
		}

		result.SHA = sha
		result.Success = true
		results = append(results, result)
	}

	return results, nil
}

// WorkPush pushes branches for selected work session repositories.
// If repoNames is empty, pushes all repos in the session.
func (m *RepositoryManager) WorkPush(ws *work.WorkSession, repoNames []string) ([]WorkPushResult, error) {
	targets := repoNames
	if len(targets) == 0 {
		targets = ws.RepoNames()
	}

	results := make([]WorkPushResult, 0, len(targets))

	for _, name := range targets {
		result := WorkPushResult{RepoName: name}

		if !ws.HasRepo(name) {
			result.Error = fmt.Errorf("repository %s is not in the work session", name)
			results = append(results, result)
			continue
		}

		repo, ok := m.config.GetRepository(name)
		if !ok {
			result.Error = fmt.Errorf("repository not found in config")
			results = append(results, result)
			continue
		}

		repoPath := m.getRepoPath(repo)

		// Check if branch exists on remote to decide push mode
		remoteExists, err := downloader.RemoteBranchExists(repoPath, ws.Branch)
		if err != nil {
			// Fall back to push with upstream
			remoteExists = false
		}

		if remoteExists {
			err = downloader.Push(repoPath, ws.Branch)
		} else {
			err = downloader.PushWithUpstream(repoPath, ws.Branch)
		}

		if err != nil {
			result.Error = err
			results = append(results, result)
			continue
		}

		result.Success = true
		results = append(results, result)
	}

	return results, nil
}

// WorkEnd ends the work session, restoring all repos to their original branches.
func (m *RepositoryManager) WorkEnd(ws *work.WorkSession, force bool) error {
	// Check for dirty repos first (unless force)
	if !force {
		var dirty []string
		for _, wr := range ws.Repos {
			repo, ok := m.config.GetRepository(wr.Name)
			if !ok {
				continue
			}
			repoPath := m.getRepoPath(repo)
			if !downloader.Exists(repoPath) {
				continue
			}
			isDirty, err := downloader.IsDirty(repoPath)
			if err != nil {
				continue
			}
			if isDirty {
				dirty = append(dirty, wr.Name)
			}
		}

		if len(dirty) > 0 {
			return fmt.Errorf("the following repositories have uncommitted changes: %v\nUse --force to end anyway, or commit/stash changes first", dirty)
		}
	}

	// Restore original branches
	for _, wr := range ws.Repos {
		repo, ok := m.config.GetRepository(wr.Name)
		if !ok {
			continue
		}
		repoPath := m.getRepoPath(repo)
		if !downloader.Exists(repoPath) {
			continue
		}

		if err := downloader.CheckoutBranch(repoPath, wr.OriginalBranch); err != nil {
			return fmt.Errorf("failed to restore branch for %s: %w", wr.Name, err)
		}
	}

	return nil
}
