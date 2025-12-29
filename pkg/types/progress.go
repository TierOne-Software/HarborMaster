package types

import "time"

// ProgressPhase represents the current phase of an operation.
type ProgressPhase string

const (
	PhaseInit       ProgressPhase = "initializing"
	PhaseConnecting ProgressPhase = "connecting"
	PhaseFetching   ProgressPhase = "fetching"
	PhaseCheckout   ProgressPhase = "checkout"
	PhaseComplete   ProgressPhase = "complete"
	PhaseFailed     ProgressPhase = "failed"
)

// ProgressUpdate is the internal progress message from downloaders.
type ProgressUpdate struct {
	Phase        ProgressPhase
	BytesTotal   int64
	BytesDone    int64
	ObjectsTotal int
	ObjectsDone  int
	Message      string
	Error        error
}

// ProgressMsg is the rich progress message for UI display.
type ProgressMsg struct {
	RepoName    string
	RepoURL     string
	Phase       ProgressPhase
	Percent     float64
	Message     string
	Error       error
	StartedAt   time.Time
	CompletedAt *time.Time
}

// IsComplete returns true if the progress indicates completion.
func (p ProgressUpdate) IsComplete() bool {
	return p.Phase == PhaseComplete || p.Phase == PhaseFailed
}

// IsComplete returns true if the progress message indicates completion.
func (p ProgressMsg) IsComplete() bool {
	return p.Phase == PhaseComplete || p.Phase == PhaseFailed
}
