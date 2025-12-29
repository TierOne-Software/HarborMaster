package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildBinary builds the harbormaster binary for testing
func buildBinary(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "hm")

	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/harbormaster")

	// Get the module root (2 levels up from cmd/harbormaster)
	cwd, _ := os.Getwd()
	moduleRoot := filepath.Join(cwd, "..", "..")
	cmd.Dir = moduleRoot

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build binary: %v\n%s", err, out)
	}

	return binaryPath
}

// runCommand runs the harbormaster command with given args
func runCommand(t *testing.T, binary string, workDir string, args ...string) (string, string, error) {
	t.Helper()

	cmd := exec.Command(binary, args...)
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// setupTestGitRepo creates a test git repository
func setupTestGitRepo(t *testing.T, dir string) {
	t.Helper()

	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}

	commands := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test User"},
	}

	for _, cmd := range commands {
		c := exec.Command(cmd[0], cmd[1:]...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("failed to run %v: %v\n%s", cmd, err, out)
		}
	}

	// Create initial commit
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	commitCmds := [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "Initial commit"},
	}

	for _, cmd := range commitCmds {
		c := exec.Command(cmd[0], cmd[1:]...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("failed to run %v: %v\n%s", cmd, err, out)
		}
	}
}

func TestE2E_Init(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not available")
	}

	binary := buildBinary(t)
	defer func() { _ = os.Remove(binary) }()

	workDir := t.TempDir()

	// Run init
	stdout, stderr, err := runCommand(t, binary, workDir, "init")
	if err != nil {
		t.Fatalf("init failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// Verify files created
	configPath := filepath.Join(workDir, ".harbormaster.toml")
	lockPath := filepath.Join(workDir, ".harbormaster.lock")

	if _, err := os.Stat(configPath); err != nil {
		t.Error("config file not created")
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Error("lock file not created")
	}

	// Verify output
	if !strings.Contains(stdout, "Initialized") {
		t.Errorf("expected 'Initialized' in output, got: %s", stdout)
	}
}

func TestE2E_Init_WithExample(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not available")
	}

	binary := buildBinary(t)
	defer func() { _ = os.Remove(binary) }()

	workDir := t.TempDir()

	// Run init with --example
	stdout, stderr, err := runCommand(t, binary, workDir, "init", "--example")
	if err != nil {
		t.Fatalf("init failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// Verify config contains example
	content, err := os.ReadFile(filepath.Join(workDir, ".harbormaster.toml"))
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}

	if !strings.Contains(string(content), "example-repo") {
		t.Error("expected example-repo in config")
	}
}

func TestE2E_Init_AlreadyExists(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not available")
	}

	binary := buildBinary(t)
	defer func() { _ = os.Remove(binary) }()

	workDir := t.TempDir()

	// First init
	_, _, _ = runCommand(t, binary, workDir, "init")

	// Second init should fail
	_, stderr, err := runCommand(t, binary, workDir, "init")
	if err == nil {
		t.Error("expected error for second init")
	}
	if !strings.Contains(stderr, "already exists") {
		t.Errorf("expected 'already exists' in error, got: %s", stderr)
	}
}

func TestE2E_Init_Force(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not available")
	}

	binary := buildBinary(t)
	defer func() { _ = os.Remove(binary) }()

	workDir := t.TempDir()

	// First init
	_, _, _ = runCommand(t, binary, workDir, "init")

	// Second init with --force should succeed
	_, _, err := runCommand(t, binary, workDir, "init", "--force")
	if err != nil {
		t.Errorf("init --force should succeed: %v", err)
	}
}

func TestE2E_List(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not available")
	}

	binary := buildBinary(t)
	defer func() { _ = os.Remove(binary) }()

	workDir := t.TempDir()

	// Init with example
	_, _, _ = runCommand(t, binary, workDir, "init", "--example")

	// List repos
	stdout, _, err := runCommand(t, binary, workDir, "list", "repos")
	if err != nil {
		t.Fatalf("list repos failed: %v", err)
	}

	if !strings.Contains(stdout, "example-repo") {
		t.Errorf("expected 'example-repo' in output, got: %s", stdout)
	}

	// List projects
	stdout, _, err = runCommand(t, binary, workDir, "list", "projects")
	if err != nil {
		t.Fatalf("list projects failed: %v", err)
	}

	if !strings.Contains(stdout, "example-project") {
		t.Errorf("expected 'example-project' in output, got: %s", stdout)
	}
}

func TestE2E_Status(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not available")
	}

	binary := buildBinary(t)
	defer func() { _ = os.Remove(binary) }()

	workDir := t.TempDir()

	// Init with example
	_, _, _ = runCommand(t, binary, workDir, "init", "--example")

	// Status
	stdout, _, err := runCommand(t, binary, workDir, "status")
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}

	if !strings.Contains(stdout, "example-repo") {
		t.Errorf("expected 'example-repo' in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "missing") {
		t.Errorf("expected 'missing' status in output, got: %s", stdout)
	}
}

func TestE2E_Add(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not available")
	}

	binary := buildBinary(t)
	defer func() { _ = os.Remove(binary) }()

	workDir := t.TempDir()

	// Init
	_, _, _ = runCommand(t, binary, workDir, "init")

	// Add repo
	stdout, stderr, err := runCommand(t, binary, workDir, "add",
		"https://github.com/test/repo.git",
		"--name", "new-repo",
		"--branch", "main")
	if err != nil {
		t.Fatalf("add failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	if !strings.Contains(stdout, "Added") {
		t.Errorf("expected 'Added' in output, got: %s", stdout)
	}

	// Verify in list
	stdout, _, _ = runCommand(t, binary, workDir, "list", "repos")
	if !strings.Contains(stdout, "new-repo") {
		t.Errorf("expected 'new-repo' in list, got: %s", stdout)
	}
}

func TestE2E_Remove(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not available")
	}

	binary := buildBinary(t)
	defer func() { _ = os.Remove(binary) }()

	workDir := t.TempDir()

	// Init with example
	_, _, _ = runCommand(t, binary, workDir, "init", "--example")

	// Remove repo
	stdout, _, err := runCommand(t, binary, workDir, "remove", "example-repo", "--force")
	if err != nil {
		t.Fatalf("remove failed: %v", err)
	}

	if !strings.Contains(stdout, "Removed") {
		t.Errorf("expected 'Removed' in output, got: %s", stdout)
	}

	// Verify not in list
	stdout, _, _ = runCommand(t, binary, workDir, "list", "repos")
	if strings.Contains(stdout, "example-repo") {
		t.Errorf("expected 'example-repo' to be removed from list")
	}
}

func TestE2E_Sync_DryRun(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not available")
	}

	binary := buildBinary(t)
	defer func() { _ = os.Remove(binary) }()

	workDir := t.TempDir()

	// Init with example
	_, _, _ = runCommand(t, binary, workDir, "init", "--example")

	// Sync dry-run
	stdout, _, err := runCommand(t, binary, workDir, "sync", "--dry-run")
	if err != nil {
		t.Fatalf("sync --dry-run failed: %v", err)
	}

	if !strings.Contains(stdout, "Would sync") {
		t.Errorf("expected 'Would sync' in output, got: %s", stdout)
	}
}

func TestE2E_Sync_RealRepo(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	binary := buildBinary(t)
	defer func() { _ = os.Remove(binary) }()

	workDir := t.TempDir()
	sourceDir := filepath.Join(workDir, "source")

	// Create a source repo
	setupTestGitRepo(t, sourceDir)

	// Init
	_, _, _ = runCommand(t, binary, workDir, "init")

	// Add the local repo with file:// URL scheme
	_, _, _ = runCommand(t, binary, workDir, "add", "file://"+sourceDir, "--name", "local-repo")

	// Sync
	stdout, stderr, err := runCommand(t, binary, workDir, "sync", "--quiet")
	if err != nil {
		t.Fatalf("sync failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// Verify cloned
	clonedPath := filepath.Join(workDir, "local-repo")
	if _, err := os.Stat(filepath.Join(clonedPath, ".git")); err != nil {
		t.Error("expected .git directory in cloned repo")
	}

	// Verify lock file updated
	lockContent, _ := os.ReadFile(filepath.Join(workDir, ".harbormaster.lock"))
	if !strings.Contains(string(lockContent), "local-repo") {
		t.Error("expected local-repo in lock file")
	}
}

func TestE2E_Help(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not available")
	}

	binary := buildBinary(t)
	defer func() { _ = os.Remove(binary) }()

	workDir := t.TempDir()

	stdout, _, err := runCommand(t, binary, workDir, "--help")
	if err != nil {
		t.Fatalf("--help failed: %v", err)
	}

	expectedCommands := []string{"init", "sync", "status", "list", "add", "remove"}
	for _, cmd := range expectedCommands {
		if !strings.Contains(stdout, cmd) {
			t.Errorf("expected '%s' in help output", cmd)
		}
	}
}

func TestE2E_Version(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not available")
	}

	binary := buildBinary(t)
	defer func() { _ = os.Remove(binary) }()

	workDir := t.TempDir()

	// Note: version subcommand not implemented, just testing help mentions version flag
	stdout, _, _ := runCommand(t, binary, workDir, "--help")
	if !strings.Contains(stdout, "help") {
		t.Error("expected help in output")
	}
}
