package lockfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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

func TestLoad_EmptyFile(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), LockFileName)
	if err := os.WriteFile(lockPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(lockPath)
	if err == nil {
		t.Fatal("expected error for zero-byte lock file")
	}
}

func TestLoad_MissingVersion(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), LockFileName)
	content := `
[entry.repo1]
url = "https://github.com/test/repo1.git"
type = "git"
requested_ref = "main"
resolved_sha = "abc123"
last_synced_at = 2026-01-01T00:00:00Z
`
	if err := os.WriteFile(lockPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(lockPath)
	if err == nil {
		t.Fatal("expected error for lock file without version")
	}
}

func TestLoad_NewerVersion(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), LockFileName)
	content := "version = 99\ngenerated_at = 2026-01-01T00:00:00Z\n"
	if err := os.WriteFile(lockPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(lockPath)
	if err == nil {
		t.Fatal("expected error for lock file with newer version")
	}
}

func TestLoad_UnknownKeys(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), LockFileName)
	content := "version = 1\ngenerated_at = 2026-01-01T00:00:00Z\nfrobnicate = true\n"
	if err := os.WriteFile(lockPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(lockPath)
	if err == nil {
		t.Fatal("expected error for unknown keys in lock file")
	}
	if !strings.Contains(err.Error(), "frobnicate") {
		t.Errorf("expected error to name the unknown key, got: %v", err)
	}
}

func TestSave_PreservesExistingFileOnError(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, LockFileName)

	lf := New()
	lf.Update("repo1", LockEntry{URL: "https://example.com/1.git", Type: "git", ResolvedSHA: "abc"})
	if err := lf.Save(lockPath); err != nil {
		t.Fatalf("failed to save: %v", err)
	}
	original, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}

	if os.Geteuid() == 0 {
		t.Skip("running as root; directory permissions are not enforced")
	}
	if err := os.Chmod(tmpDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(tmpDir, 0o755) })

	if err := lf.Save(lockPath); err == nil {
		t.Fatal("expected save to fail in read-only directory")
	}

	after, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Error("existing lock file was modified by a failed save")
	}
}

func TestSave_FilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, LockFileName)

	lf := New()
	if err := lf.Save(lockPath); err != nil {
		t.Fatalf("failed to save: %v", err)
	}
	info, err := os.Stat(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("expected new file mode 0600, got %o", perm)
	}

	// Existing permissions are preserved on re-save.
	if err := os.Chmod(lockPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := lf.Save(lockPath); err != nil {
		t.Fatalf("failed to re-save: %v", err)
	}
	info, err = os.Stat(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Errorf("expected preserved file mode 0644, got %o", perm)
	}
}

func TestLockUnlock(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), LockFileName)

	l, err := Lock(lockPath)
	if err != nil {
		t.Fatalf("failed to acquire lock: %v", err)
	}
	if err := l.Unlock(); err != nil {
		t.Fatalf("failed to release lock: %v", err)
	}

	// Unlock is idempotent and nil-safe.
	if err := l.Unlock(); err != nil {
		t.Errorf("second Unlock should be a no-op, got: %v", err)
	}
	var nilLock *FileLock
	if err := nilLock.Unlock(); err != nil {
		t.Errorf("nil Unlock should be a no-op, got: %v", err)
	}

	// The lock can be re-acquired after release.
	l2, err := Lock(lockPath)
	if err != nil {
		t.Fatalf("failed to re-acquire lock: %v", err)
	}
	if err := l2.Unlock(); err != nil {
		t.Fatalf("failed to release re-acquired lock: %v", err)
	}
}

func TestMutate_Contention(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), LockFileName)

	const (
		workers    = 8
		iterations = 20
	)

	// Each worker performs read-modify-write cycles incrementing a shared
	// counter stored in the lock file. Without inter-process locking these
	// concurrent load-modify-save cycles lose updates (last writer wins).
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				err := Mutate(lockPath, func(lf *LockFile) error {
					count := 0
					if entry, ok := lf.Get("counter"); ok {
						n, err := strconv.Atoi(entry.ResolvedSHA)
						if err != nil {
							return fmt.Errorf("bad counter value %q: %w", entry.ResolvedSHA, err)
						}
						count = n
					}
					lf.Update("counter", LockEntry{
						URL:          "https://example.com/counter.git",
						Type:         "git",
						RequestedRef: "main",
						ResolvedSHA:  strconv.Itoa(count + 1),
						LastSyncedAt: time.Now(),
					})
					return nil
				})
				if err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("mutate failed: %v", err)
	}

	lf, err := Load(lockPath)
	if err != nil {
		t.Fatalf("failed to load lock file: %v", err)
	}
	entry, ok := lf.Get("counter")
	if !ok {
		t.Fatal("counter entry missing")
	}
	want := strconv.Itoa(workers * iterations)
	if entry.ResolvedSHA != want {
		t.Errorf("lost updates under contention: expected counter %s, got %s", want, entry.ResolvedSHA)
	}
}

func TestMutate_ErrorLeavesFileUnchanged(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), LockFileName)

	lf := New()
	lf.Update("repo1", LockEntry{URL: "https://example.com/1.git", Type: "git", ResolvedSHA: "abc"})
	if err := lf.Save(lockPath); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}

	sentinel := fmt.Errorf("boom")
	err = Mutate(lockPath, func(lf *LockFile) error {
		lf.Update("repo2", LockEntry{URL: "https://example.com/2.git", Type: "git"})
		return sentinel
	})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected sentinel error, got: %v", err)
	}

	after, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Error("lock file was modified even though fn returned an error")
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
