package ui

import (
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tierone/harbormaster/pkg/types"
)

// ProgressManager coordinates UI display for concurrent operations.
type ProgressManager struct {
	program     *tea.Program
	model       Model
	msgChan     chan types.ProgressMsg
	resultChan  chan types.OperationResult
	results     []types.OperationResult
	resultMu    sync.Mutex
	done        chan struct{}
	started     bool
	interactive bool
	simple      *SimpleOutput
}

// NewProgressManager creates a new UI manager.
func NewProgressManager(interactive bool) *ProgressManager {
	pm := &ProgressManager{
		model:       NewModel(),
		msgChan:     make(chan types.ProgressMsg, 100),
		resultChan:  make(chan types.OperationResult, 100),
		results:     []types.OperationResult{},
		done:        make(chan struct{}),
		interactive: interactive,
	}

	if !interactive {
		pm.simple = NewSimpleOutput()
	}

	return pm
}

// Start initializes the UI manager.
func (pm *ProgressManager) Start() error {
	if pm.started {
		return nil
	}
	pm.started = true

	if pm.interactive {
		pm.program = tea.NewProgram(pm.model)

		// Start the message processor
		go pm.processMessages()

		// Run the program in background
		go func() {
			_, _ = pm.program.Run()
		}()
	} else {
		go pm.processMessagesSimple()
	}

	return nil
}

func (pm *ProgressManager) processMessages() {
	for {
		select {
		case msg, ok := <-pm.msgChan:
			if !ok {
				return
			}
			if pm.program != nil {
				pm.program.Send(ProgressMsg(msg))
			}
		case result, ok := <-pm.resultChan:
			if !ok {
				return
			}
			pm.resultMu.Lock()
			pm.results = append(pm.results, result)
			pm.resultMu.Unlock()
		case <-pm.done:
			return
		}
	}
}

func (pm *ProgressManager) processMessagesSimple() {
	for {
		select {
		case msg, ok := <-pm.msgChan:
			if !ok {
				return
			}
			if pm.simple != nil {
				pm.simple.Update(msg)
			}
		case result, ok := <-pm.resultChan:
			if !ok {
				return
			}
			pm.resultMu.Lock()
			pm.results = append(pm.results, result)
			pm.resultMu.Unlock()
		case <-pm.done:
			return
		}
	}
}

// SendProgress sends a progress update to the UI.
func (pm *ProgressManager) SendProgress(msg types.ProgressMsg) {
	select {
	case pm.msgChan <- msg:
	default:
		// Channel full, drop message
	}
}

// SendResult sends an operation result.
func (pm *ProgressManager) SendResult(result types.OperationResult) {
	select {
	case pm.resultChan <- result:
	default:
	}
}

// Wait blocks until Complete is called and returns the sync result.
func (pm *ProgressManager) Wait() *types.SyncResult {
	<-pm.done

	pm.resultMu.Lock()
	defer pm.resultMu.Unlock()

	return types.NewSyncResult(pm.results, 0)
}

// Complete signals that all operations are complete.
func (pm *ProgressManager) Complete(duration time.Duration) {
	if pm.interactive && pm.program != nil {
		pm.program.Send(CompleteMsg{})
		// Give the UI time to render the final state
		time.Sleep(100 * time.Millisecond)
		pm.program.Quit()
	} else if pm.simple != nil {
		pm.simple.Complete()
	}

	close(pm.done)
}

// Stop gracefully shuts down the UI.
func (pm *ProgressManager) Stop() {
	close(pm.msgChan)
	close(pm.resultChan)

	if pm.program != nil {
		pm.program.Quit()
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
