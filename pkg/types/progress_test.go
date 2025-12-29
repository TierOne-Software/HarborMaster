package types

import (
	"testing"
	"time"
)

func TestProgressPhaseConstants(t *testing.T) {
	tests := []struct {
		phase    ProgressPhase
		expected string
	}{
		{PhaseInit, "initializing"},
		{PhaseConnecting, "connecting"},
		{PhaseFetching, "fetching"},
		{PhaseCheckout, "checkout"},
		{PhaseComplete, "complete"},
		{PhaseFailed, "failed"},
	}

	for _, tt := range tests {
		t.Run(string(tt.phase), func(t *testing.T) {
			if string(tt.phase) != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, tt.phase)
			}
		})
	}
}

func TestProgressUpdate_IsComplete(t *testing.T) {
	tests := []struct {
		name     string
		phase    ProgressPhase
		expected bool
	}{
		{"init is not complete", PhaseInit, false},
		{"connecting is not complete", PhaseConnecting, false},
		{"fetching is not complete", PhaseFetching, false},
		{"checkout is not complete", PhaseCheckout, false},
		{"complete is complete", PhaseComplete, true},
		{"failed is complete", PhaseFailed, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := ProgressUpdate{Phase: tt.phase}
			if p.IsComplete() != tt.expected {
				t.Errorf("expected IsComplete()=%v for phase %s", tt.expected, tt.phase)
			}
		})
	}
}

func TestProgressMsg_IsComplete(t *testing.T) {
	tests := []struct {
		name     string
		phase    ProgressPhase
		expected bool
	}{
		{"complete phase", PhaseComplete, true},
		{"failed phase", PhaseFailed, true},
		{"fetching phase", PhaseFetching, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := ProgressMsg{Phase: tt.phase}
			if p.IsComplete() != tt.expected {
				t.Errorf("expected IsComplete()=%v for phase %s", tt.expected, tt.phase)
			}
		})
	}
}

func TestProgressUpdate_WithError(t *testing.T) {
	p := ProgressUpdate{
		Phase:        PhaseFetching,
		BytesTotal:   1000,
		BytesDone:    500,
		ObjectsTotal: 10,
		ObjectsDone:  5,
		Message:      "downloading",
		Error:        nil,
	}

	if p.Phase != PhaseFetching {
		t.Errorf("expected phase %s, got %s", PhaseFetching, p.Phase)
	}
	if p.BytesTotal != 1000 {
		t.Errorf("expected BytesTotal 1000, got %d", p.BytesTotal)
	}
	if p.BytesDone != 500 {
		t.Errorf("expected BytesDone 500, got %d", p.BytesDone)
	}
}

func TestProgressMsg_Fields(t *testing.T) {
	now := time.Now()
	completed := now.Add(time.Second)

	p := ProgressMsg{
		RepoName:    "test-repo",
		RepoURL:     "https://github.com/test/repo.git",
		Phase:       PhaseComplete,
		Percent:     100.0,
		Message:     "done",
		Error:       nil,
		StartedAt:   now,
		CompletedAt: &completed,
	}

	if p.RepoName != "test-repo" {
		t.Errorf("expected RepoName 'test-repo', got '%s'", p.RepoName)
	}
	if p.Percent != 100.0 {
		t.Errorf("expected Percent 100.0, got %f", p.Percent)
	}
	if p.CompletedAt == nil {
		t.Error("expected CompletedAt to be set")
	}
}
