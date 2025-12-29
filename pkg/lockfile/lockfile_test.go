package lockfile

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	lf := New()

	if lf.Version != CurrentVersion {
		t.Errorf("expected version %d, got %d", CurrentVersion, lf.Version)
	}
	if lf.Entries == nil {
		t.Error("expected Entries to be initialized")
	}
	if len(lf.Entries) != 0 {
		t.Errorf("expected empty Entries, got %d", len(lf.Entries))
	}
}

func TestLoad_NonExistent(t *testing.T) {
	lf, err := Load("/nonexistent/path/to/lockfile")
	if err != nil {
		t.Fatalf("expected no error for nonexistent file, got: %v", err)
	}

	if lf.Version != CurrentVersion {
		t.Errorf("expected version %d, got %d", CurrentVersion, lf.Version)
	}
	if len(lf.Entries) != 0 {
		t.Errorf("expected empty Entries, got %d", len(lf.Entries))
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, ".harbormaster.lock")

	// Create and save
	lf := New()
	lf.Update("repo1", LockEntry{
		URL:          "https://github.com/test/repo1.git",
		Type:         "git",
		RequestedRef: "main",
		ResolvedSHA:  "abc123def456",
		LastSyncedAt: time.Now(),
	})

	if err := lf.Save(lockPath); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file not created: %v", err)
	}

	// Load and verify
	loaded, err := Load(lockPath)
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}

	if loaded.Version != CurrentVersion {
		t.Errorf("expected version %d, got %d", CurrentVersion, loaded.Version)
	}
	if len(loaded.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(loaded.Entries))
	}

	entry, ok := loaded.Get("repo1")
	if !ok {
		t.Fatal("expected to find repo1")
	}
	if entry.ResolvedSHA != "abc123def456" {
		t.Errorf("expected SHA 'abc123def456', got '%s'", entry.ResolvedSHA)
	}
}

func TestLockFile_Update(t *testing.T) {
	lf := New()

	// Add entry
	lf.Update("repo1", LockEntry{
		URL:         "https://github.com/test/repo1.git",
		Type:        "git",
		ResolvedSHA: "abc123",
	})

	if len(lf.Entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(lf.Entries))
	}

	// Update existing
	lf.Update("repo1", LockEntry{
		URL:         "https://github.com/test/repo1.git",
		Type:        "git",
		ResolvedSHA: "def456",
	})

	if len(lf.Entries) != 1 {
		t.Errorf("expected 1 entry after update, got %d", len(lf.Entries))
	}

	entry, _ := lf.Get("repo1")
	if entry.ResolvedSHA != "def456" {
		t.Errorf("expected updated SHA 'def456', got '%s'", entry.ResolvedSHA)
	}
}

func TestLockFile_Get(t *testing.T) {
	lf := New()
	lf.Update("repo1", LockEntry{ResolvedSHA: "abc123"})

	// Found
	entry, ok := lf.Get("repo1")
	if !ok {
		t.Error("expected to find repo1")
	}
	if entry.ResolvedSHA != "abc123" {
		t.Errorf("expected SHA 'abc123', got '%s'", entry.ResolvedSHA)
	}

	// Not found
	_, ok = lf.Get("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent")
	}
}

func TestLockFile_Remove(t *testing.T) {
	lf := New()
	lf.Update("repo1", LockEntry{ResolvedSHA: "abc123"})
	lf.Update("repo2", LockEntry{ResolvedSHA: "def456"})

	// Remove existing
	removed := lf.Remove("repo1")
	if !removed {
		t.Error("expected Remove to return true")
	}
	if len(lf.Entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(lf.Entries))
	}

	// Remove nonexistent
	removed = lf.Remove("nonexistent")
	if removed {
		t.Error("expected Remove to return false for nonexistent")
	}
}

func TestLockFile_Has(t *testing.T) {
	lf := New()
	lf.Update("repo1", LockEntry{})

	if !lf.Has("repo1") {
		t.Error("expected Has to return true for repo1")
	}
	if lf.Has("nonexistent") {
		t.Error("expected Has to return false for nonexistent")
	}
}

func TestLockFile_ShouldUpdate(t *testing.T) {
	lf := New()
	lf.Update("repo1", LockEntry{RequestedRef: "main"})

	tests := []struct {
		name         string
		repoName     string
		requestedRef string
		expected     bool
	}{
		{"same ref", "repo1", "main", false},
		{"different ref", "repo1", "develop", true},
		{"nonexistent repo", "repo2", "main", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := lf.ShouldUpdate(tt.repoName, tt.requestedRef)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestLockFile_GetResolvedSHA(t *testing.T) {
	lf := New()
	lf.Update("repo1", LockEntry{ResolvedSHA: "abc123"})

	// Found
	sha, ok := lf.GetResolvedSHA("repo1")
	if !ok {
		t.Error("expected to find SHA")
	}
	if sha != "abc123" {
		t.Errorf("expected 'abc123', got '%s'", sha)
	}

	// Not found
	_, ok = lf.GetResolvedSHA("nonexistent")
	if ok {
		t.Error("expected not to find SHA for nonexistent")
	}
}

func TestLockFile_Names(t *testing.T) {
	lf := New()
	lf.Update("repo1", LockEntry{})
	lf.Update("repo2", LockEntry{})
	lf.Update("repo3", LockEntry{})

	names := lf.Names()
	if len(names) != 3 {
		t.Errorf("expected 3 names, got %d", len(names))
	}

	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}

	for _, expected := range []string{"repo1", "repo2", "repo3"} {
		if !nameSet[expected] {
			t.Errorf("expected to find '%s' in names", expected)
		}
	}
}

func TestLockFile_Len(t *testing.T) {
	lf := New()
	if lf.Len() != 0 {
		t.Errorf("expected Len 0, got %d", lf.Len())
	}

	lf.Update("repo1", LockEntry{})
	lf.Update("repo2", LockEntry{})

	if lf.Len() != 2 {
		t.Errorf("expected Len 2, got %d", lf.Len())
	}
}

func TestLockFile_Clear(t *testing.T) {
	lf := New()
	lf.Update("repo1", LockEntry{})
	lf.Update("repo2", LockEntry{})

	lf.Clear()

	if lf.Len() != 0 {
		t.Errorf("expected Len 0 after Clear, got %d", lf.Len())
	}
}

func TestNewEntry(t *testing.T) {
	entry := NewEntry(
		"https://github.com/test/repo.git",
		"git",
		"main",
		"abc123",
	)

	if entry.URL != "https://github.com/test/repo.git" {
		t.Errorf("unexpected URL: %s", entry.URL)
	}
	if entry.Type != "git" {
		t.Errorf("unexpected Type: %s", entry.Type)
	}
	if entry.RequestedRef != "main" {
		t.Errorf("unexpected RequestedRef: %s", entry.RequestedRef)
	}
	if entry.ResolvedSHA != "abc123" {
		t.Errorf("unexpected ResolvedSHA: %s", entry.ResolvedSHA)
	}
	if entry.LastSyncedAt.IsZero() {
		t.Error("expected LastSyncedAt to be set")
	}
}

func TestNewEntryWithSubmodules(t *testing.T) {
	submodules := []SubmoduleLock{
		{Path: "vendor/lib", URL: "https://github.com/other/lib.git", ResolvedSHA: "xyz789"},
	}

	entry := NewEntryWithSubmodules(
		"https://github.com/test/repo.git",
		"git",
		"main",
		"abc123",
		submodules,
	)

	if len(entry.Submodules) != 1 {
		t.Errorf("expected 1 submodule, got %d", len(entry.Submodules))
	}
	if entry.Submodules[0].Path != "vendor/lib" {
		t.Errorf("unexpected submodule path: %s", entry.Submodules[0].Path)
	}
}
