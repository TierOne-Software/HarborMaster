package manager

import (
	"fmt"

	"github.com/tierone/harbormaster/pkg/config"
	"github.com/tierone/harbormaster/pkg/downloader"
	"github.com/tierone/harbormaster/pkg/lockfile"
)

// LockChange describes the result of a lock-file operation on one repository.
type LockChange struct {
	Name       string
	OldSHA     string // previously locked SHA, "" if there was no entry
	NewSHA     string // SHA now recorded in the lock file
	Changed    bool   // NewSHA differs from OldSHA
	Skipped    bool   // the repository is not eligible (see SkipReason)
	SkipReason string
	Warning    string // non-fatal caveat (e.g. unpushed commit adopted with force)
	Dirty      bool   // worktree had uncommitted changes when the SHA was read
	Error      error  // fatal for this repository; no lock entry was written
}

// UpdateLockToRemote updates lock entries to the latest commit on each
// repository's configured branch, resolving refs against the remote with
// 'git ls-remote'. Local checkouts are not touched. Repositories whose
// config pins a tag or commit are skipped: their lock entry follows the
// config, not a moving branch. HTTP repositories are skipped as well.
func (m *RepositoryManager) UpdateLockToRemote(filter Filter) ([]LockChange, error) {
	if m.lockFile == nil {
		return nil, fmt.Errorf("no lock file available")
	}

	repos, err := m.getRepositories(filter)
	if err != nil {
		return nil, err
	}

	changes := make([]LockChange, 0, len(repos))
	for _, repo := range repos {
		change := LockChange{Name: repo.Name}
		if entry, ok := m.lockFile.Get(repo.Name); ok {
			change.OldSHA = entry.ResolvedSHA
		}

		switch {
		case repo.Type != config.RepoTypeGit:
			change.Skipped = true
			change.SkipReason = "not a git repository"
		case repo.Commit != "" || repo.Tag != "":
			change.Skipped = true
			change.SkipReason = "config pins a commit/tag; edit the config to change it"
		default:
			branch := repo.Branch
			if branch == "" {
				branch = m.config.General.DefaultBranch
			}
			sha, err := downloader.ResolveRemoteRef(repo.URL, "refs/heads/"+branch)
			if err != nil {
				change.Error = err
				break
			}
			change.NewSHA = sha
			change.Changed = sha != change.OldSHA
			m.updateLockEntry(&repo, sha)
		}

		changes = append(changes, change)
	}
	return changes, nil
}

// UpdateLockToLocal updates lock entries to each repository's current local
// HEAD ("adopt"). The local checkout is not modified. A HEAD that is not
// contained in any remote-tracking ref cannot be reproduced by 'hm sync
// --locked' on another machine, so adopting it is refused unless force is
// true; with force the entry is written and the change carries a warning.
func (m *RepositoryManager) UpdateLockToLocal(filter Filter, force bool) ([]LockChange, error) {
	if m.lockFile == nil {
		return nil, fmt.Errorf("no lock file available")
	}

	repos, err := m.getRepositories(filter)
	if err != nil {
		return nil, err
	}

	changes := make([]LockChange, 0, len(repos))
	for _, repo := range repos {
		change := LockChange{Name: repo.Name}
		if entry, ok := m.lockFile.Get(repo.Name); ok {
			change.OldSHA = entry.ResolvedSHA
		}

		if repo.Type != config.RepoTypeGit {
			change.Skipped = true
			change.SkipReason = "not a git repository"
			changes = append(changes, change)
			continue
		}

		repoPath := m.getRepoPath(&repo)
		if !downloader.Exists(repoPath) {
			change.Error = fmt.Errorf("repository not found at %s (run 'hm sync' first)", repoPath)
			changes = append(changes, change)
			continue
		}

		sha, err := downloader.GetHeadSHA(repoPath)
		if err != nil {
			change.Error = err
			changes = append(changes, change)
			continue
		}
		change.NewSHA = sha

		if dirty, err := downloader.IsDirty(repoPath); err == nil {
			change.Dirty = dirty
		}

		published, err := downloader.RemoteContains(repoPath, sha)
		if err != nil {
			change.Error = err
			changes = append(changes, change)
			continue
		}
		if !published {
			msg := "local HEAD is not contained in any origin ref; 'hm sync --locked' cannot reproduce it elsewhere — push it first, or use --force to pin anyway"
			if !force {
				change.Error = fmt.Errorf("%s", msg)
				changes = append(changes, change)
				continue
			}
			change.Warning = msg
		}

		change.Changed = sha != change.OldSHA
		m.updateLockEntry(&repo, sha)
		changes = append(changes, change)
	}
	return changes, nil
}

// updateLockEntry writes a lock entry for the repository pinned to sha.
func (m *RepositoryManager) updateLockEntry(repo *config.Repository, sha string) {
	requestedRef := repo.GetEffectiveRef(m.config.General.DefaultBranch)
	entry := lockfile.NewEntry(
		repo.URL,
		string(repo.Type),
		requestedRef,
		sha,
	)
	m.lockFile.Update(repo.Name, entry)
}
