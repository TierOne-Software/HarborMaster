package types

import "time"

// OperationResult represents the outcome of a single repository operation.
type OperationResult struct {
	RepoName  string
	RepoURL   string
	Success   bool
	Error     error
	Duration  time.Duration
	CommitSHA string
	Branch    string
	Tag       string
}

// SyncResult aggregates results from a sync operation.
type SyncResult struct {
	TotalRepos   int
	SuccessCount int
	FailureCount int
	Results      []OperationResult
	Duration     time.Duration
}

// NewSyncResult creates a new SyncResult from a slice of operation results.
func NewSyncResult(results []OperationResult, duration time.Duration) *SyncResult {
	sr := &SyncResult{
		TotalRepos: len(results),
		Results:    results,
		Duration:   duration,
	}
	for _, r := range results {
		if r.Success {
			sr.SuccessCount++
		} else {
			sr.FailureCount++
		}
	}
	return sr
}

// HasFailures returns true if any operations failed.
func (sr *SyncResult) HasFailures() bool {
	return sr.FailureCount > 0
}

// FailedResults returns only the failed operation results.
func (sr *SyncResult) FailedResults() []OperationResult {
	var failed []OperationResult
	for _, r := range sr.Results {
		if !r.Success {
			failed = append(failed, r)
		}
	}
	return failed
}
