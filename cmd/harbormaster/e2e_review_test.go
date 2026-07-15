package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// The one-off --work-dir override must never be written back to the config
// file by a command that saves it.
func TestE2E_WorkDirOverride_NotPersisted(t *testing.T) {
	workDir := t.TempDir()
	configPath := filepath.Join(workDir, ".harbormaster.toml")

	mustRun(t, workDir, "init")
	before := workDirLine(t, configPath)

	override := t.TempDir()
	mustRun(t, workDir, "--work-dir", override, "add", "https://example.com/x.git", "--name", "x", "--branch", "main")

	after := workDirLine(t, configPath)
	if after != before {
		t.Errorf("--work-dir override was persisted: work_dir changed from %q to %q", before, after)
	}
	if strings.Contains(after, override) {
		t.Errorf("config work_dir contains the override path: %s", after)
	}
}

func workDirLine(t *testing.T, configPath string) string {
	t.Helper()
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "work_dir") {
			return strings.TrimSpace(line)
		}
	}
	t.Fatalf("no work_dir line in config:\n%s", raw)
	return ""
}

// Read-only and repair commands must still run on a config that fails strict
// validation, surfacing the problem as a warning; mutating commands refuse.
func TestE2E_BrokenConfig_RelaxedCommands(t *testing.T) {
	workDir := t.TempDir()
	configPath := filepath.Join(workDir, ".harbormaster.toml")

	mustRun(t, workDir, "init")
	mustRun(t, workDir, "add", "https://example.com/x.git", "--name", "x", "--branch", "main")

	// Introduce a key strict loading rejects.
	f, err := os.OpenFile(configPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\n[bogus_section]\nbogus_key = true\n"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	// status and list still work, with a warning on stderr.
	stdout, stderr, err := runCommand(t, workDir, "status")
	if err != nil {
		t.Fatalf("status should succeed on a broken config: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stderr, "warning:") || !strings.Contains(stderr, "bogus_section") {
		t.Errorf("expected a warning naming bogus_section on stderr, got: %s", stderr)
	}
	if !strings.Contains(stdout, "x") {
		t.Errorf("status should list the repository, got: %s", stdout)
	}
	if _, _, err := runCommand(t, workDir, "list", "repos", "--json"); err != nil {
		t.Errorf("list repos should succeed on a broken config: %v", err)
	}

	// Strict commands refuse until the config is fixed.
	if _, stderr, err := runCommand(t, workDir, "sync", "--dry-run"); err == nil {
		t.Error("sync should refuse a config with unknown keys")
	} else if !strings.Contains(stderr, "bogus_section") {
		t.Errorf("sync error should name the unknown key, got: %s", stderr)
	}

	// remove (a repair command) works on the broken config; its save
	// rewrites the file from the parsed structure, which drops the unknown
	// section — so strict commands accept the config again.
	if _, _, err := runCommand(t, workDir, "remove", "x", "--force"); err != nil {
		t.Errorf("remove should succeed on a broken config: %v", err)
	}
	if _, stderr, err := runCommand(t, workDir, "sync", "--dry-run"); err != nil {
		t.Errorf("sync should accept the repaired config: %v\nstderr: %s", err, stderr)
	}
}

// A corrupt lock file must not brick the workspace: read-only commands warn,
// strict commands point at the repair path, and init --force performs it.
func TestE2E_CorruptLockFile_Recovery(t *testing.T) {
	requireGit(t)

	workDir := t.TempDir()
	setupSyncedWorkspace(t, workDir, "repo1")
	lockPath := filepath.Join(workDir, ".harbormaster.lock")

	// Simulate a truncated lock file from a crashed save.
	if err := os.WriteFile(lockPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	// status still works, with a warning.
	if _, stderr, err := runCommand(t, workDir, "status"); err != nil {
		t.Fatalf("status should succeed with a corrupt lock file: %v\nstderr: %s", err, stderr)
	} else if !strings.Contains(stderr, "lock file") {
		t.Errorf("expected a lock file warning on stderr, got: %s", stderr)
	}

	// sync refuses, pointing at the repair path.
	if _, stderr, err := runCommand(t, workDir, "sync", "--quiet"); err == nil {
		t.Error("sync should fail on a corrupt lock file")
	} else if !strings.Contains(stderr, "init --force") {
		t.Errorf("sync error should mention 'init --force', got: %s", stderr)
	}

	// init --force resets the lock file and keeps a backup.
	stdout := mustRun(t, workDir, "init", "--force")
	if !strings.Contains(stdout, "corrupt") {
		t.Errorf("init --force should report the corrupt lock file, got: %s", stdout)
	}
	if _, err := os.Stat(lockPath + ".corrupt"); err != nil {
		t.Error("expected a .corrupt backup of the bad lock file")
	}

	// The workspace is fully usable again.
	mustRun(t, workDir, "status")
}

// Porcelain output lists every applicable state so scripts keying on one
// state keep seeing it when several apply.
func TestE2E_Porcelain_MultiState(t *testing.T) {
	requireGit(t)

	workDir := t.TempDir()
	setupSyncedWorkspace(t, workDir, "repo1")

	// Make the checkout dirty and drop the lock entry so the repo is
	// simultaneously dirty and outdated.
	clonedPath := filepath.Join(workDir, "repo1")
	if err := os.WriteFile(filepath.Join(clonedPath, "README.md"), []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(workDir, ".harbormaster.lock")); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := runCommand(t, workDir, "status", "--porcelain")
	if err != nil {
		t.Fatalf("status --porcelain failed: %v", err)
	}
	if !strings.Contains(stdout, "dirty,outdated") {
		t.Errorf("porcelain should report both states, got: %s", stdout)
	}
}

// sync --locked must never replace a pinned HTTP artifact with content that
// does not match the locked hash, and must not re-download a matching one.
func TestE2E_LockedSync_HTTPPreservesPinnedFile(t *testing.T) {
	var mu sync.Mutex
	content := "artifact v1"
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		requests++
		_, _ = w.Write([]byte(content))
	}))
	defer server.Close()

	workDir := t.TempDir()
	mustRun(t, workDir, "init")
	mustRun(t, workDir, "add", server.URL+"/asset.bin", "--name", "asset", "--type", "http")
	mustRun(t, workDir, "sync", "--quiet")

	assetPath := filepath.Join(workDir, "asset")
	if got, _ := os.ReadFile(assetPath); string(got) != "artifact v1" {
		t.Fatalf("unexpected artifact content after sync: %q", got)
	}

	// Locked sync with matching content must not re-download.
	mu.Lock()
	before := requests
	mu.Unlock()
	mustRun(t, workDir, "sync", "--locked", "--quiet")
	mu.Lock()
	after := requests
	mu.Unlock()
	if after != before {
		t.Errorf("locked sync re-downloaded a matching artifact (%d extra request(s))", after-before)
	}

	// Upstream changes. The local artifact still matches the lock, so a
	// locked sync succeeds without touching it or the network.
	mu.Lock()
	content = "artifact v2 (tampered)"
	before = requests
	mu.Unlock()
	mustRun(t, workDir, "sync", "--locked", "--quiet")
	if got, _ := os.ReadFile(assetPath); string(got) != "artifact v1" {
		t.Errorf("locked sync replaced the pinned artifact: %q", got)
	}
	mu.Lock()
	after = requests
	mu.Unlock()
	if after != before {
		t.Errorf("locked sync contacted the server despite a matching artifact (%d request(s))", after-before)
	}

	// With the local artifact gone, the locked sync must re-download — and
	// must reject the tampered content rather than installing it.
	if err := os.Remove(assetPath); err != nil {
		t.Fatal(err)
	}
	if _, stderr, err := runCommand(t, workDir, "sync", "--locked", "--quiet"); err == nil {
		t.Error("locked sync should fail when upstream cannot satisfy the pinned checksum")
	} else if !strings.Contains(stderr, "checksum mismatch") {
		t.Errorf("expected a checksum mismatch error, got: %s", stderr)
	}
	if got, err := os.ReadFile(assetPath); err == nil && strings.Contains(string(got), "tampered") {
		t.Errorf("tampered content was installed: %q", got)
	}
}
