package lockfile

import (
	"fmt"
	"os"
	"time"

	"github.com/BurntSushi/toml"
)

const (
	// LockFileName is the name of the lock file.
	LockFileName = ".harbormaster.lock"

	// CurrentVersion is the current lock file format version.
	CurrentVersion = 1
)

// LockFile represents the lock file for reproducible syncs.
type LockFile struct {
	Version     int                  `toml:"version"`
	GeneratedAt time.Time            `toml:"generated_at"`
	Entries     map[string]LockEntry `toml:"entry"`
	path        string
}

// New creates a new empty lock file.
func New() *LockFile {
	return &LockFile{
		Version:     CurrentVersion,
		GeneratedAt: time.Now(),
		Entries:     make(map[string]LockEntry),
	}
}

// Load reads an existing lock file.
func Load(path string) (*LockFile, error) {
	lf := &LockFile{
		Entries: make(map[string]LockEntry),
		path:    path,
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		// Return empty lock file if file doesn't exist
		lf.Version = CurrentVersion
		lf.GeneratedAt = time.Now()
		return lf, nil
	}

	if _, err := toml.DecodeFile(path, lf); err != nil {
		return nil, fmt.Errorf("failed to parse lock file: %w", err)
	}

	lf.path = path
	return lf, nil
}

// Save writes the lock file to disk.
func (lf *LockFile) Save(path string) error {
	lf.GeneratedAt = time.Now()

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create lock file: %w", err)
	}

	// Write header comment
	if _, err := f.WriteString("# Harbormaster Lock File\n"); err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.WriteString("# DO NOT EDIT - This file is auto-generated\n"); err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.WriteString("# Use 'hm sync' to update\n\n"); err != nil {
		_ = f.Close()
		return err
	}

	encoder := toml.NewEncoder(f)
	if err := encoder.Encode(lf); err != nil {
		_ = f.Close()
		return fmt.Errorf("failed to encode lock file: %w", err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to write lock file: %w", err)
	}

	lf.path = path
	return nil
}

// Path returns the path to the lock file.
func (lf *LockFile) Path() string {
	return lf.path
}

// Update updates or adds an entry for a repository.
func (lf *LockFile) Update(name string, entry LockEntry) {
	if lf.Entries == nil {
		lf.Entries = make(map[string]LockEntry)
	}
	lf.Entries[name] = entry
}

// Get retrieves the lock entry for a repository.
func (lf *LockFile) Get(name string) (LockEntry, bool) {
	entry, ok := lf.Entries[name]
	return entry, ok
}

// Remove removes an entry from the lock file.
func (lf *LockFile) Remove(name string) bool {
	if _, ok := lf.Entries[name]; ok {
		delete(lf.Entries, name)
		return true
	}
	return false
}

// Has returns true if an entry exists for the repository.
func (lf *LockFile) Has(name string) bool {
	_, ok := lf.Entries[name]
	return ok
}

// ShouldUpdate returns true if the repository needs updating.
// This is true if:
// - The entry doesn't exist
// - The requested ref has changed
func (lf *LockFile) ShouldUpdate(name string, requestedRef string) bool {
	entry, ok := lf.Entries[name]
	if !ok {
		return true
	}
	return entry.RequestedRef != requestedRef
}

// GetResolvedSHA returns the resolved SHA for a repository, if locked.
func (lf *LockFile) GetResolvedSHA(name string) (string, bool) {
	entry, ok := lf.Entries[name]
	if !ok {
		return "", false
	}
	return entry.ResolvedSHA, true
}

// Names returns all repository names in the lock file.
func (lf *LockFile) Names() []string {
	names := make([]string, 0, len(lf.Entries))
	for name := range lf.Entries {
		names = append(names, name)
	}
	return names
}

// Len returns the number of entries in the lock file.
func (lf *LockFile) Len() int {
	return len(lf.Entries)
}

// Clear removes all entries from the lock file.
func (lf *LockFile) Clear() {
	lf.Entries = make(map[string]LockEntry)
}

// NewEntry creates a new LockEntry with the given parameters.
func NewEntry(url, repoType, requestedRef, resolvedSHA string) LockEntry {
	return LockEntry{
		URL:          url,
		Type:         repoType,
		RequestedRef: requestedRef,
		ResolvedSHA:  resolvedSHA,
		LastSyncedAt: time.Now(),
	}
}

// NewEntryWithSubmodules creates a new LockEntry with submodules.
func NewEntryWithSubmodules(url, repoType, requestedRef, resolvedSHA string, submodules []SubmoduleLock) LockEntry {
	entry := NewEntry(url, repoType, requestedRef, resolvedSHA)
	entry.Submodules = submodules
	return entry
}
