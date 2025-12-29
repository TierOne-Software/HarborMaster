package types

import (
	"errors"
	"testing"
	"time"
)

func TestOperationResult(t *testing.T) {
	result := OperationResult{
		RepoName:  "test-repo",
		RepoURL:   "https://github.com/test/repo.git",
		Success:   true,
		Error:     nil,
		Duration:  5 * time.Second,
		CommitSHA: "abc123def456",
		Branch:    "main",
		Tag:       "",
	}

	if result.RepoName != "test-repo" {
		t.Errorf("expected RepoName 'test-repo', got '%s'", result.RepoName)
	}
	if !result.Success {
		t.Error("expected Success to be true")
	}
	if result.Duration != 5*time.Second {
		t.Errorf("expected Duration 5s, got %v", result.Duration)
	}
}

func TestNewSyncResult(t *testing.T) {
	results := []OperationResult{
		{RepoName: "repo1", Success: true},
		{RepoName: "repo2", Success: true},
		{RepoName: "repo3", Success: false, Error: errors.New("failed")},
	}

	sr := NewSyncResult(results, 10*time.Second)

	if sr.TotalRepos != 3 {
		t.Errorf("expected TotalRepos 3, got %d", sr.TotalRepos)
	}
	if sr.SuccessCount != 2 {
		t.Errorf("expected SuccessCount 2, got %d", sr.SuccessCount)
	}
	if sr.FailureCount != 1 {
		t.Errorf("expected FailureCount 1, got %d", sr.FailureCount)
	}
	if sr.Duration != 10*time.Second {
		t.Errorf("expected Duration 10s, got %v", sr.Duration)
	}
}

func TestSyncResult_HasFailures(t *testing.T) {
	tests := []struct {
		name     string
		results  []OperationResult
		expected bool
	}{
		{
			name: "no failures",
			results: []OperationResult{
				{Success: true},
				{Success: true},
			},
			expected: false,
		},
		{
			name: "has failures",
			results: []OperationResult{
				{Success: true},
				{Success: false},
			},
			expected: true,
		},
		{
			name:     "empty results",
			results:  []OperationResult{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sr := NewSyncResult(tt.results, time.Second)
			if sr.HasFailures() != tt.expected {
				t.Errorf("expected HasFailures()=%v, got %v", tt.expected, sr.HasFailures())
			}
		})
	}
}

func TestSyncResult_FailedResults(t *testing.T) {
	err := errors.New("test error")
	results := []OperationResult{
		{RepoName: "repo1", Success: true},
		{RepoName: "repo2", Success: false, Error: err},
		{RepoName: "repo3", Success: true},
		{RepoName: "repo4", Success: false, Error: err},
	}

	sr := NewSyncResult(results, time.Second)
	failed := sr.FailedResults()

	if len(failed) != 2 {
		t.Errorf("expected 2 failed results, got %d", len(failed))
	}

	expectedNames := map[string]bool{"repo2": true, "repo4": true}
	for _, f := range failed {
		if !expectedNames[f.RepoName] {
			t.Errorf("unexpected failed repo: %s", f.RepoName)
		}
	}
}

func TestSyncResult_AllSuccess(t *testing.T) {
	results := []OperationResult{
		{RepoName: "repo1", Success: true},
		{RepoName: "repo2", Success: true},
	}

	sr := NewSyncResult(results, time.Second)

	if sr.HasFailures() {
		t.Error("expected no failures")
	}
	if len(sr.FailedResults()) != 0 {
		t.Error("expected empty failed results")
	}
	if sr.SuccessCount != 2 {
		t.Errorf("expected SuccessCount 2, got %d", sr.SuccessCount)
	}
}
