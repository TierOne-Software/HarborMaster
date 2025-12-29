package manager

import (
	"fmt"
	"sync"
	"time"

	"github.com/tierone/harbormaster/pkg/config"
	"github.com/tierone/harbormaster/pkg/downloader"
	"github.com/tierone/harbormaster/pkg/lockfile"
	"github.com/tierone/harbormaster/pkg/types"
	"github.com/tierone/harbormaster/pkg/ui"
)

// Sync synchronizes all or selected repositories.
func (m *RepositoryManager) Sync(filter Filter) (*types.SyncResult, error) {
	startTime := time.Now()

	// Ensure work directory exists
	if err := m.ensureWorkDir(); err != nil {
		return nil, fmt.Errorf("failed to create work directory: %w", err)
	}

	// Get repositories to sync
	repos, err := m.getRepositories(filter)
	if err != nil {
		return nil, err
	}

	if len(repos) == 0 {
		return &types.SyncResult{}, nil
	}

	// Create UI if not provided
	if m.ui == nil {
		m.ui = ui.NewProgressManager(m.interactive)
		if err := m.ui.Start(); err != nil {
			return nil, fmt.Errorf("failed to start UI: %w", err)
		}
	}

	// Create semaphore for concurrency control
	sem := newSemaphore(m.concurrent)
	var wg sync.WaitGroup
	results := make([]types.OperationResult, len(repos))

	// Sync repositories concurrently
	for i, repo := range repos {
		wg.Add(1)
		go func(idx int, r config.Repository) {
			defer wg.Done()
			sem.acquire()
			defer sem.release()

			results[idx] = m.syncRepository(&r)
		}(i, repo)
	}

	// Wait for all operations to complete
	wg.Wait()

	// Update lock file
	m.updateLockFile(results)

	duration := time.Since(startTime)
	m.ui.Complete(duration)

	return types.NewSyncResult(results, duration), nil
}

// Status returns the status of all or selected repositories.
func (m *RepositoryManager) Status(filter Filter) ([]RepoStatus, error) {
	repos, err := m.getRepositories(filter)
	if err != nil {
		return nil, err
	}

	statuses := make([]RepoStatus, 0, len(repos))

	for _, repo := range repos {
		status := m.getRepoStatus(&repo)
		statuses = append(statuses, status)
	}

	return statuses, nil
}

func (m *RepositoryManager) getRepoStatus(repo *config.Repository) RepoStatus {
	repoPath := m.getRepoPath(repo)
	requestedRef := repo.GetEffectiveRef(m.config.General.DefaultBranch)

	status := RepoStatus{
		Name:         repo.Name,
		Path:         repoPath,
		RequestedRef: requestedRef,
	}

	// Check if repository exists
	if !downloader.Exists(repoPath) {
		status.Exists = false
		status.NeedsUpdate = true
		return status
	}
	status.Exists = true

	// Get detailed status based on repository type
	switch repo.Type {
	case config.RepoTypeGit:
		if sha, err := downloader.GetRemoteURL(repoPath); err == nil {
			_ = sha // URL check passed
		}

		// Get current SHA
		dl := downloader.NewGitDownloader(downloader.Options{})
		if sha, err := dl.GetCurrentRef(repoPath); err == nil {
			status.CurrentSHA = sha
		} else {
			status.Error = err
		}

		// Get current branch
		if branch, err := downloader.GetCurrentBranch(repoPath); err == nil {
			status.Branch = branch
		}

		// Check if dirty
		if dirty, err := downloader.IsDirty(repoPath); err == nil {
			status.IsDirty = dirty
		}
	case config.RepoTypeHTTP:
		// For HTTP, get content hash
		dl := downloader.NewHTTPDownloader(downloader.Options{})
		if hash, err := dl.GetCurrentRef(repoPath); err == nil {
			status.CurrentSHA = hash
		}
	}

	// Check lock file
	if m.lockFile != nil {
		if entry, ok := m.lockFile.Get(repo.Name); ok {
			status.LockedSHA = entry.ResolvedSHA
			status.NeedsUpdate = status.CurrentSHA != entry.ResolvedSHA
		} else {
			status.NeedsUpdate = true
		}
	}

	return status
}

// Add adds a new repository to the configuration.
func (m *RepositoryManager) Add(repo config.Repository) error {
	// Set defaults
	if repo.Type == "" {
		repo.Type = downloader.DetectType(repo.URL)
	}

	if repo.Path == "" {
		repo.Path = repo.Name
	}

	// Validate
	if repo.Name == "" {
		return fmt.Errorf("repository name is required")
	}

	if repo.URL == "" {
		return fmt.Errorf("repository URL is required")
	}

	// Add to config
	if err := m.config.AddRepository(repo); err != nil {
		return err
	}

	return nil
}

// Remove removes a repository from the configuration.
func (m *RepositoryManager) Remove(name string) error {
	// Remove from config
	if err := m.config.RemoveRepository(name); err != nil {
		return err
	}

	// Remove from lock file
	if m.lockFile != nil {
		m.lockFile.Remove(name)
	}

	return nil
}

// AddProject adds a new project to the configuration.
func (m *RepositoryManager) AddProject(proj config.Project) error {
	// Validate name
	if proj.Name == "" {
		return fmt.Errorf("project name is required")
	}

	// Validate that all referenced repos exist
	for _, repoName := range proj.Repositories {
		if _, ok := m.config.GetRepository(repoName); !ok {
			return fmt.Errorf("repository not found: %s", repoName)
		}
	}

	return m.config.AddProject(proj)
}

// RemoveProject removes a project from the configuration.
func (m *RepositoryManager) RemoveProject(name string) error {
	return m.config.RemoveProject(name)
}

// AddRepoToProject adds a repository to an existing project.
func (m *RepositoryManager) AddRepoToProject(projectName, repoName string) error {
	return m.config.AddRepoToProject(projectName, repoName)
}

// RemoveRepoFromProject removes a repository from a project.
func (m *RepositoryManager) RemoveRepoFromProject(projectName, repoName string) error {
	return m.config.RemoveRepoFromProject(projectName, repoName)
}

// SyncOne syncs a single repository by name.
func (m *RepositoryManager) SyncOne(name string) (*types.OperationResult, error) {
	repo, ok := m.config.GetRepository(name)
	if !ok {
		return nil, fmt.Errorf("repository not found: %s", name)
	}

	// Ensure work directory exists
	if err := m.ensureWorkDir(); err != nil {
		return nil, fmt.Errorf("failed to create work directory: %w", err)
	}

	result := m.syncRepository(repo)

	// Update lock file
	if result.Success && m.lockFile != nil && !m.locked {
		m.updateLockFile([]types.OperationResult{result})
	}

	return &result, nil
}

// GetConfig returns the current configuration.
func (m *RepositoryManager) GetConfig() *config.Config {
	return m.config
}

// GetLockFile returns the current lock file.
func (m *RepositoryManager) GetLockFile() *lockfile.LockFile {
	return m.lockFile
}

// SetLockFile sets the lock file.
func (m *RepositoryManager) SetLockFile(lf *lockfile.LockFile) {
	m.lockFile = lf
}

// SaveConfig saves the configuration to disk.
func (m *RepositoryManager) SaveConfig() error {
	return m.config.Save()
}

// SaveLockFile saves the lock file to disk.
func (m *RepositoryManager) SaveLockFile(path string) error {
	if m.lockFile == nil {
		return nil
	}
	return m.lockFile.Save(path)
}
