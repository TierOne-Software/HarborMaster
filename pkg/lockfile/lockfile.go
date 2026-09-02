package lockfile

import (
	"fmt"
	"os"
	"strings"
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

	md, err := toml.DecodeFile(path, lf)
	if err != nil {
		return nil, fmt.Errorf("failed to parse lock file: %w", err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, len(undecoded))
		for i, k := range undecoded {
			keys[i] = k.String()
		}
		return nil, fmt.Errorf("unknown key(s) in lock file %s: %s", path, strings.Join(keys, ", "))
	}

	// An empty or truncated file decodes without error but leaves Version at
	// zero; treat that (and unknown future versions) as an error instead of
	// silently loading an empty lock file.
	if lf.Version < 1 {
		return nil, fmt.Errorf("lock file %s has invalid version %d (file may be corrupt or truncated)", path, lf.Version)
	}
	if lf.Version > CurrentVersion {
		return nil, fmt.Errorf("lock file %s has version %d, but this version of harbormaster only supports up to version %d", path, lf.Version, CurrentVersion)
	}

	lf.path = path
	return lf, nil
}

// Save writes the lock file to disk. The file is written atomically (temp
// file + rename), so an existing lock file is never truncated by a failed
// save.
//
// Save does not acquire the inter-process lock; use Lock/Mutate to guard
// read-modify-write cycles against concurrent harbormaster processes.
func (lf *LockFile) Save(path string) error {
	lf.GeneratedAt = time.Now()

	err := writeFileAtomic(path, func(f *os.File) error {
		// Write header comment
		header := "# Harbormaster Lock File\n" +
			"# DO NOT EDIT - This file is auto-generated\n" +
			"# Use 'hm lock update', 'hm lock adopt', or 'hm sync' to update\n\n"
		if _, err := f.WriteString(header); err != nil {
			return err
		}
		if err := toml.NewEncoder(f).Encode(lf); err != nil {
			return fmt.Errorf("failed to encode lock file: %w", err)
		}
		return nil
	})
	if err != nil {
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
