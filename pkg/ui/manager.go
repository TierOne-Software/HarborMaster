package ui

import (
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tierone/harbormaster/pkg/types"
)

// ProgressManager coordinates UI display for concurrent operations.
//
// Lifecycle: Start -> SendProgress (any goroutine) -> Complete.
// After Complete, the manager can be restarted with Start for a
// subsequent run (e.g. a second Sync on the same RepositoryManager).
type ProgressManager struct {
	mu          sync.Mutex
	program     *tea.Program
	programDone chan struct{}
	msgChan     chan types.ProgressMsg
	drained     chan struct{}
	started     bool
	completed   bool
	interactive bool
	simple      *SimpleOutput
}

// NewProgressManager creates a new UI manager.
func NewProgressManager(interactive bool) *ProgressManager {
	return &ProgressManager{
		interactive: interactive,
	}
}

// Start initializes the UI manager. Calling Start on an already-running
// manager is a no-op; calling it after Complete restarts the manager.
func (pm *ProgressManager) Start() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.started && !pm.completed {
		return nil
	}

	pm.started = true
	pm.completed = false
	pm.msgChan = make(chan types.ProgressMsg, 100)
	pm.drained = make(chan struct{})

	if pm.interactive {
		pm.program = tea.NewProgram(NewModel())
		pm.programDone = make(chan struct{})

		// Run the program in the background. programDone is closed whenever
		// Run returns, including when it fails to start (e.g. no TTY), which
		// is the only reliable way to wait on it: tea.Program.Wait can block
		// forever if Run bailed out before initializing.
		program := pm.program
		programDone := pm.programDone
		go func() {
			defer close(programDone)
			_, _ = program.Run()
		}()

		go pm.processMessages(pm.msgChan, pm.drained, program)
	} else {
		pm.simple = NewSimpleOutput()
		go pm.processMessagesSimple(pm.msgChan, pm.drained, pm.simple)
	}

	return nil
}

// processMessages forwards progress messages to the Bubbletea program until
// the message channel is closed, then signals that all messages have been
// delivered.
func (pm *ProgressManager) processMessages(msgChan <-chan types.ProgressMsg, drained chan<- struct{}, program *tea.Program) {
	for msg := range msgChan {
		program.Send(ProgressMsg(msg))
	}
	close(drained)
}

// processMessagesSimple forwards progress messages to the simple output until
// the message channel is closed, then signals that all messages have been
// delivered.
func (pm *ProgressManager) processMessagesSimple(msgChan <-chan types.ProgressMsg, drained chan<- struct{}, simple *SimpleOutput) {
	for msg := range msgChan {
		simple.Update(msg)
	}
	close(drained)
}

// SendProgress sends a progress update to the UI.
//
// Terminal messages (completed/failed, or any message carrying an error) are
// never dropped: they determine the final success/failure rendering.
// Intermediate updates (e.g. percentage ticks) may be dropped if the UI
// cannot keep up.
func (pm *ProgressManager) SendProgress(msg types.ProgressMsg) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if !pm.started || pm.completed {
		return
	}

	if msg.IsComplete() || msg.Error != nil {
		// Blocking send: the processor goroutine is draining the channel,
		// and Complete cannot close it while we hold the mutex.
		pm.msgChan <- msg
		return
	}

	select {
	case pm.msgChan <- msg:
	default:
		// Channel full: dropping an intermediate update is harmless.
	}
}

// Complete signals that all operations are complete. It drains all pending
// progress messages before rendering the final summary, so no terminal
// message can be lost or reordered. Calling Complete more than once is safe.
func (pm *ProgressManager) Complete(duration time.Duration) {
	_ = duration

	pm.mu.Lock()
	if !pm.started || pm.completed {
		pm.mu.Unlock()
		return
	}
	pm.completed = true
	msgChan := pm.msgChan
	drained := pm.drained
	program := pm.program
	programDone := pm.programDone
	simple := pm.simple
	pm.mu.Unlock()

	// No new messages are accepted past this point (completed is set), so
	// closing the channel is safe. Wait until the processor has delivered
	// every buffered message before rendering the summary.
	close(msgChan)
	<-drained

	if pm.interactive && program != nil {
		// The completion message is sequenced strictly after all progress
		// messages because the channel has been fully drained. CompleteMsg
		// makes the model render the final summary and quit; Send is a no-op
		// if the program already exited (or never managed to start).
		program.Send(CompleteMsg{})
		<-programDone
	} else if simple != nil {
		simple.Complete()
	}
}

// CreateProgressMsg creates a ProgressMsg for a repository.
func CreateProgressMsg(repoName, repoURL string, phase types.ProgressPhase, message string) types.ProgressMsg {
	return types.ProgressMsg{
		RepoName:  repoName,
		RepoURL:   repoURL,
		Phase:     phase,
		Message:   message,
		StartedAt: time.Now(),
	}
}

// CreateProgressMsgWithPercent creates a ProgressMsg with percentage.
func CreateProgressMsgWithPercent(repoName, repoURL string, phase types.ProgressPhase, percent float64, message string) types.ProgressMsg {
	return types.ProgressMsg{
		RepoName:  repoName,
		RepoURL:   repoURL,
		Phase:     phase,
		Percent:   percent,
		Message:   message,
		StartedAt: time.Now(),
	}
}

// CreateCompletedMsg creates a completed ProgressMsg.
func CreateCompletedMsg(repoName, repoURL, message string) types.ProgressMsg {
	now := time.Now()
	return types.ProgressMsg{
		RepoName:    repoName,
		RepoURL:     repoURL,
		Phase:       types.PhaseComplete,
		Message:     message,
		StartedAt:   now,
		CompletedAt: &now,
	}
}

// CreateErrorMsg creates an error ProgressMsg.
func CreateErrorMsg(repoName, repoURL string, err error) types.ProgressMsg {
	now := time.Now()
	return types.ProgressMsg{
		RepoName:    repoName,
		RepoURL:     repoURL,
		Phase:       types.PhaseFailed,
		Error:       err,
		StartedAt:   now,
		CompletedAt: &now,
	}
}
