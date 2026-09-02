package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// testBinary is the path to the harbormaster binary, built once in TestMain.
var testBinary string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "hm-e2e-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	testBinary = filepath.Join(tmpDir, "hm")

	cmd := exec.Command("go", "build", "-o", testBinary, "./cmd/harbormaster")
	cwd, _ := os.Getwd()
	cmd.Dir = filepath.Join(cwd, "..", "..")
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build binary: %v\n%s", err, out)
		_ = os.RemoveAll(tmpDir)
		os.Exit(1)
	}

	code := m.Run()
	_ = os.RemoveAll(tmpDir)
	os.Exit(code)
}

// runCommand runs the harbormaster command with given args.
func runCommand(t *testing.T, workDir string, args ...string) (string, string, error) {
	t.Helper()
	return runCommandStdin(t, workDir, "", args...)
}

// runCommandStdin runs the harbormaster command with the given stdin content.
func runCommandStdin(t *testing.T, workDir string, stdin string, args ...string) (string, string, error) {
	t.Helper()

	cmd := exec.Command(testBinary, args...)
	cmd.Dir = workDir
	cmd.Env = hermeticEnv()
	cmd.Stdin = strings.NewReader(stdin)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// mustRun runs the command and fails the test if it exits non-zero.
func mustRun(t *testing.T, workDir string, args ...string) string {
	t.Helper()

	stdout, stderr, err := runCommand(t, workDir, args...)
	if err != nil {
		t.Fatalf("command %v failed: %v\nstdout: %s\nstderr: %s", args, err, stdout, stderr)
	}
	return stdout
}

// hermeticEnv returns an environment that keeps git isolated from the
// user's global and system configuration.
func hermeticEnv() []string {
	return append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
}

// listProjectNames returns the project names from 'list projects --json'.
func listProjectNames(t *testing.T, workDir string) []string {
	t.Helper()

	stdout := mustRun(t, workDir, "list", "projects", "--json")
	var projects []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(stdout), &projects); err != nil {
		t.Fatalf("list projects --json produced invalid JSON: %v\noutput: %s", err, stdout)
	}
	names := make([]string, 0, len(projects))
	for _, p := range projects {
		names = append(names, p.Name)
	}
	return names
}

// gitIn runs a git command in dir and fails the test on error.
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()

	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = hermeticEnv()
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// requireGit skips the test if git is not available.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
}

// setupTestGitRepo creates a local git repository with an initial commit
// on the 'main' branch and returns its path.
func setupTestGitRepo(t *testing.T, dir string) {
	t.Helper()

	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}

	gitIn(t, dir, "init")
	gitIn(t, dir, "symbolic-ref", "HEAD", "refs/heads/main")
	gitIn(t, dir, "config", "user.email", "test@test.com")
	gitIn(t, dir, "config", "user.name", "Test User")

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-m", "Initial commit")
}

// commitFileIn adds a commit with the given file to an existing test repo.
func commitFileIn(t *testing.T, dir, name, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-m", "Add "+name)
}

// setupSyncedWorkspace initializes a workspace with one local git repo
// (named repoName) that has been synced. Returns the source repo path.
func setupSyncedWorkspace(t *testing.T, workDir, repoName string) string {
	t.Helper()

	sourceDir := filepath.Join(workDir, "sources", repoName)
	setupTestGitRepo(t, sourceDir)

	mustRun(t, workDir, "init")
	mustRun(t, workDir, "add", "file://"+sourceDir, "--name", repoName, "--branch", "main")
	mustRun(t, workDir, "sync", "--quiet")

	return sourceDir
}

// ---------------------------------------------------------------------------
// init
// ---------------------------------------------------------------------------

func TestE2E_Init(t *testing.T) {
	workDir := t.TempDir()

	stdout := mustRun(t, workDir, "init")

	if _, err := os.Stat(filepath.Join(workDir, ".harbormaster.toml")); err != nil {
		t.Error("config file not created")
	}
	if _, err := os.Stat(filepath.Join(workDir, ".harbormaster.lock")); err != nil {
		t.Error("lock file not created")
	}
	if !strings.Contains(stdout, "Initialized") {
		t.Errorf("expected 'Initialized' in output, got: %s", stdout)
	}
}

func TestE2E_Init_WithExample(t *testing.T) {
	workDir := t.TempDir()

	mustRun(t, workDir, "init", "--example")

	content, err := os.ReadFile(filepath.Join(workDir, ".harbormaster.toml"))
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	if !strings.Contains(string(content), "example-repo") {
		t.Error("expected example-repo in config")
	}
}

func TestE2E_Init_AlreadyExists(t *testing.T) {
	workDir := t.TempDir()

	mustRun(t, workDir, "init")

	_, stderr, err := runCommand(t, workDir, "init")
	if err == nil {
		t.Error("expected error for second init")
	}
	if !strings.Contains(stderr, "already exists") {
		t.Errorf("expected 'already exists' in error, got: %s", stderr)
	}
}

func TestE2E_Init_Force(t *testing.T) {
	workDir := t.TempDir()

	mustRun(t, workDir, "init")
	mustRun(t, workDir, "init", "--force")
}

func TestE2E_Init_Force_PreservesLockFile(t *testing.T) {
	workDir := t.TempDir()

	mustRun(t, workDir, "init")

	// Simulate a lock file with recorded state.
	lockPath := filepath.Join(workDir, ".harbormaster.lock")
	marker := "# lock-marker-do-not-wipe\n"
	existing, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("failed to read lock file: %v", err)
	}
	if err := os.WriteFile(lockPath, append([]byte(marker), existing...), 0644); err != nil {
		t.Fatalf("failed to write lock file: %v", err)
	}

	mustRun(t, workDir, "init", "--force")

	content, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("failed to read lock file after init --force: %v", err)
	}
	if !strings.Contains(string(content), "lock-marker-do-not-wipe") {
		t.Error("init --force wiped the existing lock file")
	}
}

// ---------------------------------------------------------------------------
// list
// ---------------------------------------------------------------------------

func TestE2E_List(t *testing.T) {
	workDir := t.TempDir()

	mustRun(t, workDir, "init", "--example")

	stdout := mustRun(t, workDir, "list", "repos")
	if !strings.Contains(stdout, "example-repo") {
		t.Errorf("expected 'example-repo' in output, got: %s", stdout)
	}

	stdout = mustRun(t, workDir, "list", "projects")
	if !strings.Contains(stdout, "example-project") {
		t.Errorf("expected 'example-project' in output, got: %s", stdout)
	}
}

func TestE2E_List_FilterUnion(t *testing.T) {
	workDir := t.TempDir()

	mustRun(t, workDir, "init")
	mustRun(t, workDir, "add", "https://example.com/r1.git", "--name", "r1", "--branch", "main", "--tags", "team-a")
	mustRun(t, workDir, "add", "https://example.com/r2.git", "--name", "r2", "--branch", "main")
	mustRun(t, workDir, "add", "https://example.com/r3.git", "--name", "r3", "--branch", "main")
	mustRun(t, workDir, "project", "add", "proj", "--repos", "r2")

	// Tag filter alone
	stdout := mustRun(t, workDir, "list", "repos", "-t", "team-a")
	if !strings.Contains(stdout, "r1") || strings.Contains(stdout, "r2") {
		t.Errorf("expected only r1 for tag filter, got: %s", stdout)
	}

	// Project + tag filters combine as a union
	stdout = mustRun(t, workDir, "list", "repos", "-p", "proj", "-t", "team-a")
	if !strings.Contains(stdout, "r1") || !strings.Contains(stdout, "r2") {
		t.Errorf("expected union of r1 and r2, got: %s", stdout)
	}
	if strings.Contains(stdout, "r3") {
		t.Errorf("did not expect r3 in filtered output, got: %s", stdout)
	}
}

func TestE2E_List_SubcommandsRejectFilterFlags(t *testing.T) {
	workDir := t.TempDir()

	mustRun(t, workDir, "init", "--example")

	// 'list projects' and 'list tags' must not silently accept the repo
	// filter flags.
	if _, _, err := runCommand(t, workDir, "list", "projects", "-p", "example-project"); err == nil {
		t.Error("expected 'list projects -p' to be rejected")
	}
	if _, _, err := runCommand(t, workDir, "list", "tags", "-t", "example"); err == nil {
		t.Error("expected 'list tags -t' to be rejected")
	}
}

// ---------------------------------------------------------------------------
// status
// ---------------------------------------------------------------------------

func TestE2E_Status(t *testing.T) {
	workDir := t.TempDir()

	mustRun(t, workDir, "init", "--example")

	stdout := mustRun(t, workDir, "status")
	if !strings.Contains(stdout, "example-repo") {
		t.Errorf("expected 'example-repo' in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "missing") {
		t.Errorf("expected 'missing' status in output, got: %s", stdout)
	}
}

func TestE2E_Status_JSON(t *testing.T) {
	workDir := t.TempDir()

	mustRun(t, workDir, "init", "--example")

	stdout := mustRun(t, workDir, "status", "--json")

	var statuses []map[string]any
	if err := json.Unmarshal([]byte(stdout), &statuses); err != nil {
		t.Fatalf("status --json produced invalid JSON: %v\noutput: %s", err, stdout)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status entry, got %d", len(statuses))
	}
	if statuses[0]["name"] != "example-repo" {
		t.Errorf("expected name 'example-repo', got %v", statuses[0]["name"])
	}
}

func TestE2E_Status_JSON_EmptyWorkspace(t *testing.T) {
	workDir := t.TempDir()

	mustRun(t, workDir, "init")

	// --json must emit a valid (empty) JSON document, not a prose message.
	stdout := mustRun(t, workDir, "status", "--json")
	var statuses []map[string]any
	if err := json.Unmarshal([]byte(stdout), &statuses); err != nil {
		t.Fatalf("status --json with zero repos produced invalid JSON: %v\noutput: %s", err, stdout)
	}
	if len(statuses) != 0 {
		t.Errorf("expected empty JSON array, got %d entries", len(statuses))
	}

	// --porcelain must emit no lines and exit zero.
	stdout = mustRun(t, workDir, "status", "--porcelain")
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("expected empty porcelain output, got: %q", stdout)
	}
}

func TestE2E_Status_PorcelainMatchesTablePrecedence(t *testing.T) {
	workDir := t.TempDir()

	mustRun(t, workDir, "init", "--example")

	// The example repo does not exist on disk: both formats must report
	// the same status ("missing").
	table := mustRun(t, workDir, "status")
	porcelain := mustRun(t, workDir, "status", "--porcelain")

	if !strings.Contains(table, "missing") {
		t.Errorf("expected 'missing' in table output, got: %s", table)
	}
	if !strings.Contains(porcelain, "missing") {
		t.Errorf("expected 'missing' in porcelain output, got: %s", porcelain)
	}
}

// ---------------------------------------------------------------------------
// add / remove
// ---------------------------------------------------------------------------

func TestE2E_Add(t *testing.T) {
	workDir := t.TempDir()

	mustRun(t, workDir, "init")

	stdout := mustRun(t, workDir, "add",
		"https://github.com/test/repo.git",
		"--name", "new-repo",
		"--branch", "main")
	if !strings.Contains(stdout, "Added") {
		t.Errorf("expected 'Added' in output, got: %s", stdout)
	}

	stdout = mustRun(t, workDir, "list", "repos")
	if !strings.Contains(stdout, "new-repo") {
		t.Errorf("expected 'new-repo' in list, got: %s", stdout)
	}
}

func TestE2E_Add_InvalidTypeRejected(t *testing.T) {
	workDir := t.TempDir()

	mustRun(t, workDir, "init")

	stdout, stderr, err := runCommand(t, workDir, "add",
		"https://example.com/repo.git",
		"--name", "bad-repo",
		"--type", "banana")
	if err == nil {
		t.Fatalf("expected 'add --type banana' to fail\nstdout: %s\nstderr: %s", stdout, stderr)
	}

	// The invalid entry must not have been persisted...
	content, readErr := os.ReadFile(filepath.Join(workDir, ".harbormaster.toml"))
	if readErr != nil {
		t.Fatalf("failed to read config: %v", readErr)
	}
	if strings.Contains(string(content), "banana") {
		t.Error("invalid repository type was persisted to the config")
	}

	// ...and the workspace must still be usable.
	mustRun(t, workDir, "status")
}

func TestE2E_Remove(t *testing.T) {
	workDir := t.TempDir()

	mustRun(t, workDir, "init", "--example")

	stdout := mustRun(t, workDir, "remove", "example-repo", "--force")
	if !strings.Contains(stdout, "Removed") {
		t.Errorf("expected 'Removed' in output, got: %s", stdout)
	}

	stdout = mustRun(t, workDir, "list", "repos")
	if strings.Contains(stdout, "example-repo") {
		t.Errorf("expected 'example-repo' to be removed from list")
	}

	// The repo was referenced by example-project; the saved config must
	// still be valid and the project scrubbed.
	stdout = mustRun(t, workDir, "list", "projects", "--json")
	if strings.Contains(stdout, "example-repo") {
		t.Errorf("expected 'example-repo' to be scrubbed from projects, got: %s", stdout)
	}
}

func TestE2E_Remove_CancelledExitsNonZero(t *testing.T) {
	workDir := t.TempDir()

	mustRun(t, workDir, "init", "--example")

	// Declining the prompt (or EOF on piped stdin) must exit non-zero.
	for _, stdin := range []string{"n\n", ""} {
		_, stderr, err := runCommandStdin(t, workDir, stdin, "remove", "example-repo")
		if err == nil {
			t.Errorf("expected non-zero exit for cancelled remove (stdin %q)", stdin)
		}
		if !strings.Contains(stderr, "cancelled") {
			t.Errorf("expected 'cancelled' on stderr, got: %s", stderr)
		}
	}

	// The repo must still be configured.
	stdout := mustRun(t, workDir, "list", "repos")
	if !strings.Contains(stdout, "example-repo") {
		t.Error("cancelled remove must not modify the config")
	}
}

// ---------------------------------------------------------------------------
// project
// ---------------------------------------------------------------------------

func TestE2E_Project_Lifecycle(t *testing.T) {
	workDir := t.TempDir()

	mustRun(t, workDir, "init")
	mustRun(t, workDir, "add", "https://example.com/r1.git", "--name", "r1", "--branch", "main")
	mustRun(t, workDir, "add", "https://example.com/r2.git", "--name", "r2", "--branch", "main")

	// Create project with an initial repo
	stdout := mustRun(t, workDir, "project", "add", "proj", "--repos", "r1")
	if !strings.Contains(stdout, "Created project") {
		t.Errorf("expected 'Created project' in output, got: %s", stdout)
	}

	// Add and remove repos
	mustRun(t, workDir, "project", "add-repo", "proj", "r2")
	stdout = mustRun(t, workDir, "list", "projects", "--json")
	if !strings.Contains(stdout, "r2") {
		t.Errorf("expected r2 in project, got: %s", stdout)
	}

	mustRun(t, workDir, "project", "remove-repo", "proj", "r1")
	stdout = mustRun(t, workDir, "list", "projects", "--json")
	if strings.Contains(stdout, "r1") {
		t.Errorf("expected r1 removed from project, got: %s", stdout)
	}

	// Remove project
	mustRun(t, workDir, "project", "remove", "proj", "--force")
	if names := listProjectNames(t, workDir); len(names) != 0 {
		t.Errorf("expected project removed, got: %v", names)
	}
}

func TestE2E_Project_UnknownRepoRejected(t *testing.T) {
	workDir := t.TempDir()

	mustRun(t, workDir, "init")

	_, _, err := runCommand(t, workDir, "project", "add", "proj", "--repos", "no-such-repo")
	if err == nil {
		t.Fatal("expected project referencing unknown repo to be rejected")
	}

	// The invalid project must not have been persisted.
	if names := listProjectNames(t, workDir); len(names) != 0 {
		t.Errorf("invalid project was persisted, got: %v", names)
	}
}

// ---------------------------------------------------------------------------
// sync
// ---------------------------------------------------------------------------

func TestE2E_Sync_DryRun(t *testing.T) {
	workDir := t.TempDir()

	mustRun(t, workDir, "init", "--example")

	stdout := mustRun(t, workDir, "sync", "--dry-run")
	if !strings.Contains(stdout, "Would sync") {
		t.Errorf("expected 'Would sync' in output, got: %s", stdout)
	}
}

func TestE2E_Sync_RealRepo(t *testing.T) {
	requireGit(t)

	workDir := t.TempDir()
	sourceDir := setupSyncedWorkspace(t, workDir, "local-repo")
	_ = sourceDir

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

func TestE2E_Sync_Locked(t *testing.T) {
	requireGit(t)

	workDir := t.TempDir()
	sourceDir := setupSyncedWorkspace(t, workDir, "repo1")

	clonedPath := filepath.Join(workDir, "repo1")
	lockedSHA := gitIn(t, clonedPath, "rev-parse", "HEAD")

	// Advance the source repository past the locked SHA.
	commitFileIn(t, sourceDir, "new.txt", "new content")

	// sync --locked must keep the clone at the locked SHA.
	mustRun(t, workDir, "sync", "--locked", "--quiet")
	currentSHA := gitIn(t, clonedPath, "rev-parse", "HEAD")
	if currentSHA != lockedSHA {
		t.Errorf("sync --locked moved HEAD: locked %s, now %s", lockedSHA, currentSHA)
	}
}

func TestE2E_Sync_FilterUnion(t *testing.T) {
	workDir := t.TempDir()

	mustRun(t, workDir, "init")
	mustRun(t, workDir, "add", "https://example.com/r1.git", "--name", "r1", "--branch", "main", "--tags", "team-a")
	mustRun(t, workDir, "add", "https://example.com/r2.git", "--name", "r2", "--branch", "main")
	mustRun(t, workDir, "add", "https://example.com/r3.git", "--name", "r3", "--branch", "main")
	mustRun(t, workDir, "project", "add", "proj", "--repos", "r2")

	// Positional names, --project, and --tag combine as a union.
	stdout := mustRun(t, workDir, "sync", "--dry-run", "-t", "team-a", "-p", "proj")
	if !strings.Contains(stdout, "r1") || !strings.Contains(stdout, "r2") {
		t.Errorf("expected union of r1 (tag) and r2 (project), got: %s", stdout)
	}
	if strings.Contains(stdout, "r3") {
		t.Errorf("did not expect r3 in filtered dry-run, got: %s", stdout)
	}

	// Positional names are not discarded when flags are also given.
	stdout = mustRun(t, workDir, "sync", "--dry-run", "-t", "team-a", "r3")
	if !strings.Contains(stdout, "r1") || !strings.Contains(stdout, "r3") {
		t.Errorf("expected union of r1 (tag) and r3 (name), got: %s", stdout)
	}
}

func TestE2E_Sync_FailurePath(t *testing.T) {
	requireGit(t)

	workDir := t.TempDir()

	mustRun(t, workDir, "init")
	mustRun(t, workDir, "add", "file:///nonexistent/e2e/bad-repo.git",
		"--name", "bad-repo", "--type", "git", "--branch", "main")

	stdout, stderr, err := runCommand(t, workDir, "sync", "--quiet")
	if err == nil {
		t.Fatalf("expected sync of unreachable repo to fail\nstdout: %s\nstderr: %s", stdout, stderr)
	}

	// Failure details and the summary must go to stderr, even with --quiet.
	if !strings.Contains(stderr, "bad-repo") {
		t.Errorf("expected failing repo name on stderr, got: %s", stderr)
	}
	if !strings.Contains(stderr, "1 of 1 repositories failed to sync") {
		t.Errorf("expected failure summary counting 1 of 1 on stderr, got: %s", stderr)
	}

	// --quiet must keep stdout free of sync noise.
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("expected empty stdout with --quiet, got: %q", stdout)
	}
}

func TestE2E_Sync_RefusedDuringWorkSession(t *testing.T) {
	requireGit(t)

	workDir := t.TempDir()
	setupSyncedWorkspace(t, workDir, "repo1")

	mustRun(t, workDir, "work", "start", "feature-x")

	_, stderr, err := runCommand(t, workDir, "sync", "--quiet")
	if err == nil {
		t.Fatal("expected sync to be refused while a work session is active")
	}
	if !strings.Contains(stderr, "work session") {
		t.Errorf("expected work session mention in error, got: %s", stderr)
	}

	// --force overrides the guard.
	mustRun(t, workDir, "sync", "--force", "--dry-run")
}

// ---------------------------------------------------------------------------
// work
// ---------------------------------------------------------------------------

func TestE2E_Work_StartCreatesSession(t *testing.T) {
	requireGit(t)

	workDir := t.TempDir()
	setupSyncedWorkspace(t, workDir, "repo1")

	stdout := mustRun(t, workDir, "work", "start", "feature-1")
	if !strings.Contains(stdout, "feature-1") {
		t.Errorf("expected session name in output, got: %s", stdout)
	}

	if _, err := os.Stat(filepath.Join(workDir, ".harbormaster.work")); err != nil {
		t.Error("expected .harbormaster.work to be created")
	}

	branch := gitIn(t, filepath.Join(workDir, "repo1"), "rev-parse", "--abbrev-ref", "HEAD")
	if branch != "feature-1" {
		t.Errorf("expected repo on branch feature-1, got: %s", branch)
	}
}

func TestE2E_Work_DoubleStartRejected(t *testing.T) {
	requireGit(t)

	workDir := t.TempDir()
	setupSyncedWorkspace(t, workDir, "repo1")

	mustRun(t, workDir, "work", "start", "feature-1")

	_, stderr, err := runCommand(t, workDir, "work", "start", "feature-2")
	if err == nil {
		t.Fatal("expected second 'work start' to fail")
	}
	if !strings.Contains(stderr, "already active") {
		t.Errorf("expected 'already active' in error, got: %s", stderr)
	}
}

func TestE2E_Work_StartRejectsDirtyRepo(t *testing.T) {
	requireGit(t)

	workDir := t.TempDir()
	setupSyncedWorkspace(t, workDir, "repo1")

	// Dirty the synced repo.
	dirtyFile := filepath.Join(workDir, "repo1", "dirty.txt")
	if err := os.WriteFile(dirtyFile, []byte("uncommitted"), 0644); err != nil {
		t.Fatalf("failed to dirty repo: %v", err)
	}
	gitIn(t, filepath.Join(workDir, "repo1"), "add", "dirty.txt")

	_, stderr, err := runCommand(t, workDir, "work", "start", "feature-1")
	if err == nil {
		t.Fatal("expected 'work start' with dirty repo to fail")
	}
	if !strings.Contains(stderr, "uncommitted") {
		t.Errorf("expected 'uncommitted' in error, got: %s", stderr)
	}
	if _, err := os.Stat(filepath.Join(workDir, ".harbormaster.work")); err == nil {
		t.Error("no session file should be created on failed start")
	}
}

func TestE2E_Work_Status_JSON(t *testing.T) {
	requireGit(t)

	workDir := t.TempDir()
	setupSyncedWorkspace(t, workDir, "repo1")

	mustRun(t, workDir, "work", "start", "feature-1")

	stdout := mustRun(t, workDir, "work", "status", "--json")

	var status struct {
		Name   string `json:"name"`
		Branch string `json:"branch"`
		Repos  []struct {
			Name       string `json:"name"`
			Branch     string `json:"branch"`
			IsOnBranch bool   `json:"is_on_branch"`
		} `json:"repos"`
	}
	if err := json.Unmarshal([]byte(stdout), &status); err != nil {
		t.Fatalf("work status --json produced invalid JSON: %v\noutput: %s", err, stdout)
	}
	if status.Branch != "feature-1" {
		t.Errorf("expected branch feature-1, got: %s", status.Branch)
	}
	if len(status.Repos) != 1 || status.Repos[0].Name != "repo1" {
		t.Errorf("expected one repo 'repo1' in session, got: %+v", status.Repos)
	}
}

func TestE2E_Work_EndRestoresBranches(t *testing.T) {
	requireGit(t)

	workDir := t.TempDir()
	setupSyncedWorkspace(t, workDir, "repo1")

	repoPath := filepath.Join(workDir, "repo1")
	beforeSHA := gitIn(t, repoPath, "rev-parse", "HEAD")

	mustRun(t, workDir, "work", "start", "feature-1")
	mustRun(t, workDir, "work", "end")

	// The repository must be back on its pre-session commit and off the
	// session branch.
	afterSHA := gitIn(t, repoPath, "rev-parse", "HEAD")
	if afterSHA != beforeSHA {
		t.Errorf("expected HEAD restored to %s, got %s", beforeSHA, afterSHA)
	}
	branch := gitIn(t, repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	if branch == "feature-1" {
		t.Error("expected repo to leave the session branch after 'work end'")
	}
	if _, err := os.Stat(filepath.Join(workDir, ".harbormaster.work")); err == nil {
		t.Error("expected .harbormaster.work to be removed after 'work end'")
	}
}

func TestE2E_Work_EndRejectsDirty_ForceOverrides(t *testing.T) {
	requireGit(t)

	workDir := t.TempDir()
	setupSyncedWorkspace(t, workDir, "repo1")

	mustRun(t, workDir, "work", "start", "feature-1")

	// Dirty the repo mid-session.
	dirtyFile := filepath.Join(workDir, "repo1", "wip.txt")
	if err := os.WriteFile(dirtyFile, []byte("wip"), 0644); err != nil {
		t.Fatalf("failed to dirty repo: %v", err)
	}
	gitIn(t, filepath.Join(workDir, "repo1"), "add", "wip.txt")

	_, stderr, err := runCommand(t, workDir, "work", "end")
	if err == nil {
		t.Fatal("expected 'work end' with dirty repo to fail")
	}
	if !strings.Contains(stderr, "uncommitted") {
		t.Errorf("expected 'uncommitted' in error, got: %s", stderr)
	}

	mustRun(t, workDir, "work", "end", "--force")
	if _, err := os.Stat(filepath.Join(workDir, ".harbormaster.work")); err == nil {
		t.Error("expected session file removed after 'work end --force'")
	}
}

func TestE2E_Work_CommitAllWithArgsRejected(t *testing.T) {
	requireGit(t)

	workDir := t.TempDir()
	setupSyncedWorkspace(t, workDir, "repo1")

	mustRun(t, workDir, "work", "start", "feature-1")

	_, stderr, err := runCommand(t, workDir, "work", "commit", "-m", "msg", "--all", "repo1")
	if err == nil {
		t.Fatal("expected 'work commit --all repo1' to be rejected")
	}
	if !strings.Contains(stderr, "cannot combine --all") {
		t.Errorf("expected conflict error, got: %s", stderr)
	}

	_, stderr, err = runCommand(t, workDir, "work", "push", "--all", "repo1")
	if err == nil {
		t.Fatal("expected 'work push --all repo1' to be rejected")
	}
	if !strings.Contains(stderr, "cannot combine --all") {
		t.Errorf("expected conflict error, got: %s", stderr)
	}
}

func TestE2E_Work_CommitAll(t *testing.T) {
	requireGit(t)

	workDir := t.TempDir()
	setupSyncedWorkspace(t, workDir, "repo1")

	mustRun(t, workDir, "work", "start", "feature-1")

	repoPath := filepath.Join(workDir, "repo1")
	if err := os.WriteFile(filepath.Join(repoPath, "feature.txt"), []byte("feature"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	stdout := mustRun(t, workDir, "work", "commit", "-m", "Add feature", "--all")
	if !strings.Contains(stdout, "repo1") {
		t.Errorf("expected repo1 in commit output, got: %s", stdout)
	}

	subject := gitIn(t, repoPath, "log", "-1", "--format=%s")
	if subject != "Add feature" {
		t.Errorf("expected commit 'Add feature', got: %s", subject)
	}
}

// ---------------------------------------------------------------------------
// misc / regression
// ---------------------------------------------------------------------------

func TestE2E_CustomConfigPath(t *testing.T) {
	workDir := t.TempDir()

	mustRun(t, workDir, "init")

	// Copy the config to a name that does not end in .harbormaster.toml;
	// this used to panic with a slice bounds error.
	content, err := os.ReadFile(filepath.Join(workDir, ".harbormaster.toml"))
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "custom.toml"), content, 0644); err != nil {
		t.Fatalf("failed to write custom config: %v", err)
	}

	stdout, stderr, err := runCommand(t, workDir, "-c", "custom.toml", "status")
	if err != nil {
		t.Fatalf("'hm -c custom.toml status' failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if strings.Contains(stderr, "panic") {
		t.Fatalf("panic with custom config path: %s", stderr)
	}
}

func TestE2E_CompletionWithoutWorkspace(t *testing.T) {
	// completion must work in a directory with no config file.
	workDir := t.TempDir()

	for _, shell := range []string{"bash", "zsh"} {
		stdout, stderr, err := runCommand(t, workDir, "completion", shell)
		if err != nil {
			t.Errorf("'hm completion %s' failed outside a workspace: %v\nstderr: %s", shell, err, stderr)
		}
		if strings.TrimSpace(stdout) == "" {
			t.Errorf("expected completion script on stdout for %s", shell)
		}
	}
}

func TestE2E_Help(t *testing.T) {
	workDir := t.TempDir()

	stdout := mustRun(t, workDir, "--help")

	expectedCommands := []string{"init", "sync", "status", "list", "add", "remove", "work", "project", "lock"}
	for _, cmd := range expectedCommands {
		if !strings.Contains(stdout, cmd) {
			t.Errorf("expected '%s' in help output", cmd)
		}
	}
}

func TestE2E_Version(t *testing.T) {
	workDir := t.TempDir()

	stdout := mustRun(t, workDir, "--version")
	if !strings.Contains(stdout, "dev") {
		t.Errorf("expected version 'dev' in output, got: %s", stdout)
	}
}

func TestE2E_NoColorFlag(t *testing.T) {
	workDir := t.TempDir()

	mustRun(t, workDir, "init", "--example")

	// --no-color must be accepted and produce output free of ANSI escapes.
	stdout := mustRun(t, workDir, "--no-color", "status")
	if strings.Contains(stdout, "\x1b[") {
		t.Errorf("expected no ANSI escapes with --no-color, got: %q", stdout)
	}
}

// ---------------------------------------------------------------------------
// lock
// ---------------------------------------------------------------------------

func lockFileContains(t *testing.T, workDir, sha string) bool {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(workDir, ".harbormaster.lock"))
	if err != nil {
		t.Fatalf("failed to read lock file: %v", err)
	}
	return strings.Contains(string(content), sha)
}

func TestE2E_LockUpdate(t *testing.T) {
	requireGit(t)
	workDir := t.TempDir()
	sourceDir := setupSyncedWorkspace(t, workDir, "repo")
	checkoutDir := filepath.Join(workDir, "repo")
	syncedSHA := gitIn(t, checkoutDir, "rev-parse", "HEAD")

	// Advance the source branch.
	commitFileIn(t, sourceDir, "new.txt", "new content")
	newSHA := gitIn(t, sourceDir, "rev-parse", "HEAD")

	// --dry-run reports the change but does not write the lock file.
	stdout := mustRun(t, workDir, "lock", "update", "--dry-run")
	if !strings.Contains(stdout, "Dry run") {
		t.Errorf("expected dry-run notice, got: %s", stdout)
	}
	if lockFileContains(t, workDir, newSHA) {
		t.Error("dry-run must not write the new SHA to the lock file")
	}

	// The real run updates the lock file but not the checkout.
	stdout = mustRun(t, workDir, "lock", "update")
	if !strings.Contains(stdout, "->") {
		t.Errorf("expected old -> new output, got: %s", stdout)
	}
	if !lockFileContains(t, workDir, newSHA) {
		t.Error("expected lock file to contain the new branch tip")
	}
	if got := gitIn(t, checkoutDir, "rev-parse", "HEAD"); got != syncedSHA {
		t.Errorf("lock update moved the checkout to %s, want untouched %s", got, syncedSHA)
	}

	// Status now reports drift: checkout behind the lock.
	stdout = mustRun(t, workDir, "status")
	if !strings.Contains(stdout, "drift") {
		t.Errorf("expected drift after lock update, got: %s", stdout)
	}

	// --sync moves the checkout to the new pin.
	mustRun(t, workDir, "lock", "update", "--sync", "--quiet")
	if got := gitIn(t, checkoutDir, "rev-parse", "HEAD"); got != newSHA {
		t.Errorf("lock update --sync left checkout at %s, want %s", got, newSHA)
	}
}

func TestE2E_LockAdopt(t *testing.T) {
	requireGit(t)
	workDir := t.TempDir()
	setupSyncedWorkspace(t, workDir, "repo")
	checkoutDir := filepath.Join(workDir, "repo")

	// Commit locally without pushing.
	commitFileIn(t, checkoutDir, "local.txt", "local content")
	localSHA := gitIn(t, checkoutDir, "rev-parse", "HEAD")

	// Unpushed HEAD is refused without --force.
	_, stderr, err := runCommand(t, workDir, "lock", "adopt")
	if err == nil {
		t.Error("expected 'lock adopt' to refuse an unpushed HEAD")
	}
	if !strings.Contains(stderr, "--force") {
		t.Errorf("expected refusal to mention --force, got: %s", stderr)
	}
	if lockFileContains(t, workDir, localSHA) {
		t.Error("lock file must not change when adopt is refused")
	}

	// --force pins the local HEAD, with a warning.
	_, stderr, err = runCommand(t, workDir, "lock", "adopt", "--force")
	if err != nil {
		t.Fatalf("lock adopt --force failed: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stderr, "warning") {
		t.Errorf("expected a warning when force-adopting an unpushed HEAD, got: %s", stderr)
	}
	if !lockFileContains(t, workDir, localSHA) {
		t.Error("expected lock file to contain the local HEAD SHA")
	}

	// Status agrees the checkout matches the lock.
	stdout := mustRun(t, workDir, "status")
	if !strings.Contains(stdout, "locked") {
		t.Errorf("expected locked status after adopt, got: %s", stdout)
	}
}
