package work

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	ws := New("my-feature", "feature/my-feature")

	if ws.Version != CurrentVersion {
		t.Errorf("expected version %d, got %d", CurrentVersion, ws.Version)
	}
	if ws.Name != "my-feature" {
		t.Errorf("expected name 'my-feature', got '%s'", ws.Name)
	}
	if ws.Branch != "feature/my-feature" {
		t.Errorf("expected branch 'feature/my-feature', got '%s'", ws.Branch)
	}
	if len(ws.Repos) != 0 {
		t.Errorf("expected 0 repos, got %d", len(ws.Repos))
	}
}

func TestAddRepo(t *testing.T) {
	ws := New("test", "test-branch")

	err := ws.AddRepo(WorkRepo{
		Name:           "repo1",
		OriginalBranch: "main",
		OriginalSHA:    "abc123",
		AddedAt:        time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ws.Repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(ws.Repos))
	}

	// Duplicate should fail
	err = ws.AddRepo(WorkRepo{Name: "repo1"})
	if err == nil {
		t.Fatal("expected error for duplicate repo")
	}
}

func TestRemoveRepo(t *testing.T) {
	ws := New("test", "test-branch")
	_ = ws.AddRepo(WorkRepo{Name: "repo1"})
	_ = ws.AddRepo(WorkRepo{Name: "repo2"})

	err := ws.RemoveRepo("repo1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ws.Repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(ws.Repos))
	}
	if ws.Repos[0].Name != "repo2" {
		t.Errorf("expected remaining repo 'repo2', got '%s'", ws.Repos[0].Name)
	}

	// Removing non-existent should fail
	err = ws.RemoveRepo("repo3")
	if err == nil {
		t.Fatal("expected error for non-existent repo")
	}
}

func TestGetRepo(t *testing.T) {
	ws := New("test", "test-branch")
	_ = ws.AddRepo(WorkRepo{Name: "repo1", OriginalBranch: "main"})

	r, ok := ws.GetRepo("repo1")
	if !ok {
		t.Fatal("expected to find repo1")
	}
	if r.OriginalBranch != "main" {
		t.Errorf("expected original branch 'main', got '%s'", r.OriginalBranch)
	}

	_, ok = ws.GetRepo("nonexistent")
	if ok {
		t.Fatal("expected not to find nonexistent repo")
	}
}

func TestHasRepo(t *testing.T) {
	ws := New("test", "test-branch")
	_ = ws.AddRepo(WorkRepo{Name: "repo1"})

	if !ws.HasRepo("repo1") {
		t.Error("expected HasRepo to return true for repo1")
	}
	if ws.HasRepo("repo2") {
		t.Error("expected HasRepo to return false for repo2")
	}
}

func TestRepoNames(t *testing.T) {
	ws := New("test", "test-branch")
	_ = ws.AddRepo(WorkRepo{Name: "alpha"})
	_ = ws.AddRepo(WorkRepo{Name: "beta"})
	_ = ws.AddRepo(WorkRepo{Name: "gamma"})

	names := ws.RepoNames()
	if len(names) != 3 {
		t.Fatalf("expected 3 names, got %d", len(names))
	}
	if names[0] != "alpha" || names[1] != "beta" || names[2] != "gamma" {
		t.Errorf("unexpected names: %v", names)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, WorkFileName)

	ws := New("feature-x", "feature/x")
	_ = ws.AddRepo(WorkRepo{
		Name:           "service-a",
		OriginalBranch: "main",
		OriginalSHA:    "abc123def456",
		AddedAt:        time.Date(2026, 4, 6, 10, 0, 0, 0, time.UTC),
	})
	_ = ws.AddRepo(WorkRepo{
		Name:           "service-b",
		OriginalBranch: "develop",
		OriginalSHA:    "789abc012def",
		AddedAt:        time.Date(2026, 4, 6, 10, 1, 0, 0, time.UTC),
	})

	// Save
	if err := ws.Save(path); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	// Load
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}
	if loaded == nil {
		t.Fatal("loaded session is nil")
	}

	if loaded.Version != CurrentVersion {
		t.Errorf("version: expected %d, got %d", CurrentVersion, loaded.Version)
	}
	if loaded.Name != "feature-x" {
		t.Errorf("name: expected 'feature-x', got '%s'", loaded.Name)
	}
	if loaded.Branch != "feature/x" {
		t.Errorf("branch: expected 'feature/x', got '%s'", loaded.Branch)
	}
	if len(loaded.Repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(loaded.Repos))
	}
	if loaded.Repos[0].Name != "service-a" {
		t.Errorf("repo 0: expected 'service-a', got '%s'", loaded.Repos[0].Name)
	}
	if loaded.Repos[0].OriginalBranch != "main" {
		t.Errorf("repo 0 branch: expected 'main', got '%s'", loaded.Repos[0].OriginalBranch)
	}
	if loaded.Repos[1].Name != "service-b" {
		t.Errorf("repo 1: expected 'service-b', got '%s'", loaded.Repos[1].Name)
	}
}

func TestLoadNonExistent(t *testing.T) {
	ws, err := Load("/nonexistent/path/.harbormaster.work")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ws != nil {
		t.Fatal("expected nil session for non-existent file")
	}
}

func TestFileExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, WorkFileName)

	if FileExists(path) {
		t.Error("expected FileExists to return false for non-existent file")
	}

	ws := New("test", "test")
	_ = ws.Save(path)

	if !FileExists(path) {
		t.Error("expected FileExists to return true after saving")
	}
}

func TestDelete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, WorkFileName)

	ws := New("test", "test")
	_ = ws.Save(path)

	if !FileExists(path) {
		t.Fatal("file should exist after save")
	}

	if err := ws.Delete(); err != nil {
		t.Fatalf("unexpected error on delete: %v", err)
	}

	if FileExists(path) {
		t.Error("file should not exist after delete")
	}

	// Double delete should not error
	if err := ws.Delete(); err != nil {
		t.Fatalf("unexpected error on second delete: %v", err)
	}
}

func TestDeleteNoPath(t *testing.T) {
	ws := New("test", "test")
	// No path set, delete should be a no-op
	if err := ws.Delete(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSaveCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, WorkFileName)

	ws := New("test", "test-branch")
	if err := ws.Save(path); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	// Verify file contents include header
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	content := string(data)
	if !contains(content, "Harbormaster Work Session") {
		t.Error("expected header comment in file")
	}
	if !contains(content, "hm work end") {
		t.Error("expected 'hm work end' hint in file")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
