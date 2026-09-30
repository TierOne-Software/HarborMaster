package manager

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tierone/harbormaster/pkg/config"
	"github.com/tierone/harbormaster/pkg/downloader"
	"github.com/tierone/harbormaster/pkg/lockfile"
	"github.com/tierone/harbormaster/pkg/types"
	"github.com/tierone/harbormaster/pkg/ui"
)

// RepositoryManager coordinates all repository operations.
type RepositoryManager struct {
	config      *config.Config
	lockFile    *lockfile.LockFile
	ui          *ui.ProgressManager
	workDir     string
	concurrent  int
	locked      bool // If true, only sync to locked SHAs
	interactive bool
}

// ManagerOption configures the manager.
type ManagerOption func(*RepositoryManager)

// WithLockFile sets the lock file for reproducible syncs.
func WithLockFile(lf *lockfile.LockFile) ManagerOption {
	return func(m *RepositoryManager) {
		m.lockFile = lf
	}
}

// WithConcurrency sets max concurrent operations.
func WithConcurrency(n int) ManagerOption {
	return func(m *RepositoryManager) {
		if n > 0 {
			m.concurrent = n
		}
	}
}

// WithUI enables the progress UI.
func WithUI(ui *ui.ProgressManager) ManagerOption {
	return func(m *RepositoryManager) {
		m.ui = ui
	}
}

// WithLocked enables locked mode (sync only to locked SHAs).
func WithLocked(locked bool) ManagerOption {
	return func(m *RepositoryManager) {
		m.locked = locked
	}
}

// WithInteractive enables interactive UI mode.
func WithInteractive(interactive bool) ManagerOption {
	return func(m *RepositoryManager) {
		m.interactive = interactive
	}
}

// NewRepositoryManager creates a new manager.
func NewRepositoryManager(cfg *config.Config, opts ...ManagerOption) *RepositoryManager {
	m := &RepositoryManager{
		config:      cfg,
		workDir:     cfg.General.WorkDir,
		concurrent:  4,
		interactive: true,
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// Filter defines which repositories to operate on.
type Filter struct {
	Names    []string // Specific repository names
	Projects []string // Project names (expands to repos)
	Tags     []string // Filter by tags
	All      bool     // All repositories
}

// RepoStatus represents the status of a repository.
type RepoStatus struct {
	Name         string
	Path         string
	Exists       bool
	CurrentSHA   string
	LockedSHA    string
	RequestedRef string
	Branch       string
	IsDirty      bool
	NeedsUpdate  bool
	Error        error
}

// getRepositories returns the repositories matching the filter.
func (m *RepositoryManager) getRepositories(filter Filter) ([]config.Repository, error) {
	if filter.All || (len(filter.Names) == 0 && len(filter.Projects) == 0 && len(filter.Tags) == 0) {
		return m.config.Repositories, nil
	}

	repoSet := make(map[string]config.Repository)

	// Add specific repos
	for _, name := range filter.Names {
		repo, ok := m.config.GetRepository(name)
		if !ok {
			return nil, fmt.Errorf("repository not found: %s", name)
		}
		repoSet[name] = *repo
	}

	// Add repos from projects
	for _, projName := range filter.Projects {
		repos, err := m.config.GetRepositoriesForProject(projName)
		if err != nil {
			return nil, err
		}
		for _, repo := range repos {
			repoSet[repo.Name] = repo
		}
	}

	// Add repos by tag
	for _, tag := range filter.Tags {
		repos := m.config.GetRepositoriesByTag(tag)
		for _, repo := range repos {
			repoSet[repo.Name] = repo
		}
	}

	result := make([]config.Repository, 0, len(repoSet))
	for _, repo := range repoSet {
		result = append(result, repo)
	}

	return result, nil
}

// getRepoPath returns the full path for a repository.
func (m *RepositoryManager) getRepoPath(repo *config.Repository) string {
	return filepath.Join(m.workDir, repo.GetEffectivePath())
}

// shortSHA returns an abbreviated SHA for display. It is safe to call with
// SHAs shorter than 8 characters (e.g. from a corrupt lock file) and with
// the empty string.
func shortSHA(sha string) string {
	if sha == "" {
		return "(unknown)"
	}
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

// GetRepoPath returns the full path for a repository.
func (m *RepositoryManager) GetRepoPath(repo *config.Repository) string {
	return m.getRepoPath(repo)
}

// syncRepository syncs a single repository.
func (m *RepositoryManager) syncRepository(repo *config.Repository) types.OperationResult {
	startTime := time.Now()
	repoPath := m.getRepoPath(repo)

	result := types.OperationResult{
		RepoName: repo.Name,
		RepoURL:  repo.URL,
		Branch:   repo.Branch,
		Tag:      repo.Tag,
	}

	// Send initial progress
	if m.ui != nil {
		m.ui.SendProgress(ui.CreateProgressMsg(
			repo.Name, repo.URL,
			types.PhaseInit, "Starting...",
		))
	}

	// Check if we should use locked SHA
	var targetSHA string
	if m.locked && m.lockFile != nil {
		sha, ok := m.lockFile.GetResolvedSHA(repo.Name)
		if !ok {
			result.Error = fmt.Errorf("no lock entry for repository (run sync without --locked first)")
			result.Duration = time.Since(startTime)
			if m.ui != nil {
				m.ui.SendProgress(ui.CreateErrorMsg(repo.Name, repo.URL, result.Error))
			}
			return result
		}
		targetSHA = sha
	}

	// Create downloader. In locked mode, target the locked SHA explicitly so
	// the downloader checks out the recorded commit rather than the branch
	// tip (the Commit option takes precedence over Branch/Tag).
	dlRepo := repo
	if targetSHA != "" {
		lockedRepo := *repo
		lockedRepo.Commit = targetSHA
		dlRepo = &lockedRepo
	}

	dl, err := downloader.NewFromRepository(dlRepo, m.config)
	if err != nil {
		result.Error = fmt.Errorf("failed to create downloader: %w", err)
		result.Duration = time.Since(startTime)
		if m.ui != nil {
			m.ui.SendProgress(ui.CreateErrorMsg(repo.Name, repo.URL, result.Error))
		}
		return result
	}

	exists := downloader.Exists(repoPath)

	var sha string
	var progressCh <-chan types.ProgressUpdate

	if exists {
		// Update existing repository
		sha, progressCh, err = dl.UpdateWithProgress(repoPath)
	} else {
		// Clone new repository
		sha, progressCh, err = dl.DownloadWithProgress(repo.URL, repoPath)
	}

	if err != nil {
		result.Error = err
		result.Duration = time.Since(startTime)
		if m.ui != nil {
			m.ui.SendProgress(ui.CreateErrorMsg(repo.Name, repo.URL, err))
		}
		return result
	}

	// Process progress updates
	for update := range progressCh {
		// A failed update is terminal: surface it to the UI as an error so
		// the repository renders as failed (never as a silent success).
		if update.Error != nil || update.Phase == types.PhaseFailed {
			updateErr := update.Error
			if updateErr == nil {
				updateErr = fmt.Errorf("sync failed: %s", update.Message)
			}
			result.Error = updateErr
			result.Duration = time.Since(startTime)
			if m.ui != nil {
				m.ui.SendProgress(ui.CreateErrorMsg(repo.Name, repo.URL, updateErr))
			}
			return result
		}

		if m.ui != nil {
			percent := 0.0
			if update.BytesTotal > 0 {
				percent = float64(update.BytesDone) / float64(update.BytesTotal) * 100
			}
			m.ui.SendProgress(ui.CreateProgressMsgWithPercent(
				repo.Name, repo.URL,
				update.Phase, percent, update.Message,
			))
		}

		if update.Phase == types.PhaseComplete {
			sha = update.Message
		}
	}

	// Get final SHA if not set
	if sha == "" {
		sha, err = dl.GetCurrentRef(repoPath)
		if err != nil {
			result.Error = fmt.Errorf("failed to get current ref: %w", err)
			result.Duration = time.Since(startTime)
			if m.ui != nil {
				m.ui.SendProgress(ui.CreateErrorMsg(repo.Name, repo.URL, result.Error))
			}
			return result
		}
	}

	// Verify locked SHA if in locked mode
	if m.locked && targetSHA != "" && sha != targetSHA {
		result.Error = fmt.Errorf("SHA mismatch: expected %s, got %s", shortSHA(targetSHA), shortSHA(sha))
		result.Duration = time.Since(startTime)
		if m.ui != nil {
			m.ui.SendProgress(ui.CreateErrorMsg(repo.Name, repo.URL, result.Error))
		}
		return result
	}

	// In locked mode also verify submodule checkouts match the recorded
	// gitlinks. The update path runs submodule update, so a mismatch here
	// means the tree could not be brought to the locked state.
	if m.locked && targetSHA != "" && repo.Type == config.RepoTypeGit &&
		repo.HasSubmodules(m.config.General.RecurseSubmodule) {
		mismatched, subErr := downloader.CheckSubmodules(repoPath)
		if subErr != nil {
			result.Error = fmt.Errorf("failed to verify submodules: %w", subErr)
		} else if len(mismatched) > 0 {
			result.Error = fmt.Errorf("submodules not at locked commits: %s", strings.Join(mismatched, ", "))
		}
		if result.Error != nil {
			result.Duration = time.Since(startTime)
			if m.ui != nil {
				m.ui.SendProgress(ui.CreateErrorMsg(repo.Name, repo.URL, result.Error))
			}
			return result
		}
	}

	result.Success = true
	result.CommitSHA = sha
	result.Duration = time.Since(startTime)

	// Send completion progress
	if m.ui != nil {
		m.ui.SendProgress(ui.CreateCompletedMsg(
			repo.Name, repo.URL,
			fmt.Sprintf("Synced at %s", shortSHA(sha)),
		))
	}

	return result
}

// updateLockFile updates the lock file with sync results.
func (m *RepositoryManager) updateLockFile(results []types.OperationResult) {
	if m.lockFile == nil || m.locked {
		return
	}

	for _, result := range results {
		if !result.Success {
			continue
		}

		repo, ok := m.config.GetRepository(result.RepoName)
		if !ok {
			continue
		}

		m.updateLockEntry(repo, result.CommitSHA)
	}
}

// ensureWorkDir creates the work directory if it doesn't exist.
func (m *RepositoryManager) ensureWorkDir() error {
	return os.MkdirAll(m.workDir, 0755)
}

// semaphore for limiting concurrent operations
type semaphore chan struct{}

func newSemaphore(n int) semaphore {
	return make(chan struct{}, n)
}

func (s semaphore) acquire() {
	s <- struct{}{}
}

func (s semaphore) release() {
	<-s
}
