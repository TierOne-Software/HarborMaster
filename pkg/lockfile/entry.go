package lockfile

import "time"

// LockEntry represents a locked repository state.
type LockEntry struct {
	URL          string          `toml:"url"`
	Type         string          `toml:"type"`
	RequestedRef string          `toml:"requested_ref"`
	ResolvedSHA  string          `toml:"resolved_sha"`
	LastSyncedAt time.Time       `toml:"last_synced_at"`
	Submodules   []SubmoduleLock `toml:"submodule,omitempty"`
}

// SubmoduleLock represents a locked submodule state.
type SubmoduleLock struct {
	Path        string `toml:"path"`
	URL         string `toml:"url"`
	ResolvedSHA string `toml:"resolved_sha"`
}

// IsStale returns true if the entry needs updating based on the requested ref.
func (e *LockEntry) IsStale(requestedRef string) bool {
	return e.RequestedRef != requestedRef
}

// Age returns the duration since the last sync.
func (e *LockEntry) Age() time.Duration {
	return time.Since(e.LastSyncedAt)
}
