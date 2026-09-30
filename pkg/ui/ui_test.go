package ui

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tierone/harbormaster/pkg/types"
)

func TestOperationState_Predicates(t *testing.T) {
	tests := []struct {
		name      string
		op        operationState
		failed    bool
		succeeded bool
	}{
		{
			name:      "complete without error succeeds",
			op:        operationState{phase: types.PhaseComplete},
			failed:    false,
			succeeded: true,
		},
		{
			name:      "failed phase with error",
			op:        operationState{phase: types.PhaseFailed, err: errors.New("boom")},
			failed:    true,
			succeeded: false,
		},
		{
			name:      "failed phase with nil error still counts as failed",
			op:        operationState{phase: types.PhaseFailed},
			failed:    true,
			succeeded: false,
		},
		{
			name:      "error carried on a non-failed phase counts as failed",
			op:        operationState{phase: types.PhaseComplete, err: errors.New("boom")},
			failed:    true,
			succeeded: false,
		},
		{
			name:      "in-flight operation is neither",
			op:        operationState{phase: types.PhaseFetching},
			failed:    false,
			succeeded: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.op.failed(); got != tt.failed {
				t.Errorf("failed() = %v, want %v", got, tt.failed)
			}
			if got := tt.op.succeeded(); got != tt.succeeded {
				t.Errorf("succeeded() = %v, want %v", got, tt.succeeded)
			}
		})
	}
}

func TestSummarizeOperations(t *testing.T) {
	ops := map[string]*operationState{
		"ok":         {phase: types.PhaseComplete},
		"failed":     {phase: types.PhaseFailed, err: errors.New("boom")},
		"failed-nil": {phase: types.PhaseFailed},
		"stuck":      {phase: types.PhaseFetching},
	}

	success, failed := summarizeOperations(ops)
	if success != 1 {
		t.Errorf("expected 1 success, got %d", success)
	}
	if failed != 3 {
		t.Errorf("expected 3 failed, got %d", failed)
	}
}

func TestModel_RunningOperationShowsMessageNotBar(t *testing.T) {
	m := NewModel()
	m.width = 120
	// A mid-clone update carrying a parsed percentage, as the git
	// downloader produces for every phase and submodule.
	m.updateOperation(ProgressMsg(CreateProgressMsgWithPercent(
		"f5-bbb-os", "url", types.PhaseFetching, 42, "Cloning into 'external/f5-iot-bbb'...",
	)))

	op := m.operations["f5-bbb-os"]
	line := m.renderOperation(op)

	if !strings.Contains(line, "Cloning into 'external/f5-iot-bbb'...") {
		t.Errorf("expected the activity message in the rendered line, got: %q", line)
	}
	if !strings.Contains(line, string(types.PhaseFetching)) {
		t.Errorf("expected the phase label in the rendered line, got: %q", line)
	}
	// The bubbles progress bar renders block glyphs; none may appear.
	if strings.ContainsAny(line, "█░") {
		t.Errorf("expected no progress bar glyphs, got: %q", line)
	}
}

func TestModel_MessageTruncatedToWidth(t *testing.T) {
	m := NewModel()
	m.width = 60
	long := strings.Repeat("x", 200)
	m.updateOperation(ProgressMsg(CreateProgressMsg("repo", "url", types.PhaseFetching, long)))

	line := m.renderOperation(m.operations["repo"])
	if strings.Contains(line, long) {
		t.Error("expected the message to be truncated to the terminal width")
	}
	if !strings.Contains(line, "...") {
		t.Errorf("expected truncation ellipsis, got: %q", line)
	}
}

func TestModel_FailedOperationRendersAsError(t *testing.T) {
	m := NewModel()
	m.updateOperation(ProgressMsg(CreateErrorMsg("repo1", "https://example.com/repo1.git", errors.New("clone exploded"))))

	op := m.operations["repo1"]
	if op == nil {
		t.Fatal("operation not tracked")
	}

	if got := m.getSymbol(op); got != SymbolError {
		t.Errorf("expected error symbol %q, got %q", SymbolError, got)
	}

	line := m.renderOperation(op)
	if !strings.Contains(line, "clone exploded") {
		t.Errorf("expected rendered line to contain error, got: %q", line)
	}
}

func TestModel_FailedPhaseWithNilErrorRendersAsError(t *testing.T) {
	m := NewModel()
	msg := CreateProgressMsgWithPercent("repo1", "url", types.PhaseFailed, 0, "something broke")
	now := time.Now()
	msg.CompletedAt = &now
	m.updateOperation(ProgressMsg(msg))

	op := m.operations["repo1"]
	if got := m.getSymbol(op); got != SymbolError {
		t.Errorf("expected error symbol for failed phase without error, got %q", got)
	}

	line := m.renderOperation(op)
	if !strings.Contains(line, "something broke") {
		t.Errorf("expected rendered line to contain failure message, got: %q", line)
	}
	if strings.Contains(line, SymbolSuccess) {
		t.Errorf("failed operation must not render the success symbol: %q", line)
	}
}

func TestModel_RenderSummary_TotalFailure(t *testing.T) {
	m := NewModel()
	m.updateOperation(ProgressMsg(CreateErrorMsg("repo1", "url", errors.New("boom"))))
	m.updateOperation(ProgressMsg(CreateErrorMsg("repo2", "url", errors.New("bang"))))

	summary := m.renderSummary()
	if strings.Contains(summary, "All") {
		t.Errorf("total failure must not report success, got: %q", summary)
	}
	if !strings.Contains(summary, "2 failed") {
		t.Errorf("expected '2 failed' in summary, got: %q", summary)
	}
	if !strings.Contains(summary, "0 synced") {
		t.Errorf("expected '0 synced' in summary, got: %q", summary)
	}
}

func TestModel_RenderSummary_Mixed(t *testing.T) {
	m := NewModel()
	m.updateOperation(ProgressMsg(CreateCompletedMsg("good", "url", "Synced at abc12345")))
	m.updateOperation(ProgressMsg(CreateErrorMsg("bad", "url", errors.New("boom"))))

	summary := m.renderSummary()
	if !strings.Contains(summary, "1 synced") {
		t.Errorf("expected '1 synced', got: %q", summary)
	}
	if !strings.Contains(summary, "1 failed") {
		t.Errorf("expected '1 failed', got: %q", summary)
	}
}

func TestModel_RenderSummary_AllSuccess(t *testing.T) {
	m := NewModel()
	m.updateOperation(ProgressMsg(CreateCompletedMsg("a", "url", "Synced at abc12345")))
	m.updateOperation(ProgressMsg(CreateCompletedMsg("b", "url", "Synced at def67890")))

	summary := m.renderSummary()
	if !strings.Contains(summary, "All 2 repositories synced successfully") {
		t.Errorf("expected all-success summary, got: %q", summary)
	}
}

func TestModel_ViewIncludesSummaryWhenDone(t *testing.T) {
	m := NewModel()
	m.updateOperation(ProgressMsg(CreateErrorMsg("repo1", "url", errors.New("boom"))))

	updated, _ := m.Update(CompleteMsg{})
	model := updated.(Model)
	if !model.done {
		t.Fatal("expected model to be done after CompleteMsg")
	}

	view := model.View()
	if !strings.Contains(view, "1 failed") {
		t.Errorf("expected final view to report failure, got: %q", view)
	}
}

func TestSimpleOutput_FailureRendering(t *testing.T) {
	var buf bytes.Buffer
	s := NewSimpleOutputTo(&buf)

	s.Update(CreateErrorMsg("repo1", "url", errors.New("boom")))
	s.Update(CreateCompletedMsg("repo2", "url", "Synced at abc12345"))
	s.Complete()

	out := buf.String()
	if !strings.Contains(out, "boom") {
		t.Errorf("expected failure output to contain error, got: %q", out)
	}
	if !strings.Contains(out, "1 synced, ✗ 1 failed") {
		t.Errorf("expected mixed summary, got: %q", out)
	}
	if strings.Contains(out, "All") {
		t.Errorf("must not claim all repositories synced, got: %q", out)
	}
}

func TestSimpleOutput_TotalFailureSummary(t *testing.T) {
	var buf bytes.Buffer
	s := NewSimpleOutputTo(&buf)

	s.Update(CreateErrorMsg("repo1", "url", errors.New("boom")))
	s.Complete()

	out := buf.String()
	if !strings.Contains(out, "0 synced, ✗ 1 failed") {
		t.Errorf("expected total-failure summary, got: %q", out)
	}
}

func TestSimpleOutput_ConcurrentUpdates(t *testing.T) {
	var buf bytes.Buffer
	s := NewSimpleOutputTo(&buf)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			name := fmt.Sprintf("repo%d", n)
			for j := 0; j < 50; j++ {
				s.Update(CreateProgressMsgWithPercent(name, "url", types.PhaseFetching, float64(j), "fetching"))
			}
			if n%2 == 0 {
				s.Update(CreateCompletedMsg(name, "url", "done"))
			} else {
				s.Update(CreateErrorMsg(name, "url", errors.New("boom")))
			}
		}(i)
	}
	wg.Wait()
	s.Complete()

	out := buf.String()
	if !strings.Contains(out, "4 synced, ✗ 4 failed") {
		t.Errorf("expected 4/4 summary, got: %q", out)
	}
}

// gateWriter blocks every Write until the gate channel is closed.
type gateWriter struct {
	gate <-chan struct{}
}

func (g *gateWriter) Write(p []byte) (int, error) {
	<-g.gate
	return len(p), nil
}

func TestProgressManager_TerminalMessagesNeverDropped(t *testing.T) {
	pm := NewProgressManager(false)
	if err := pm.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	gate := make(chan struct{})
	// Swap in a blocking writer before any messages flow so the processor
	// stalls on the first print and the channel buffer fills up.
	pm.simple.mu.Lock()
	pm.simple.w = &gateWriter{gate: gate}
	pm.simple.mu.Unlock()

	// First message triggers a print (phase change) and blocks the processor.
	pm.SendProgress(CreateProgressMsg("blocked", "url", types.PhaseFetching, "fetching"))

	// Flood with far more intermediate updates than the channel can buffer.
	// These must not block: dropping them is allowed.
	floodDone := make(chan struct{})
	go func() {
		for i := 0; i < 500; i++ {
			pm.SendProgress(CreateProgressMsgWithPercent("blocked", "url", types.PhaseFetching, float64(i%100), "fetching"))
		}
		close(floodDone)
	}()

	select {
	case <-floodDone:
	case <-time.After(5 * time.Second):
		t.Fatal("intermediate progress updates blocked instead of being dropped")
	}

	// Unblock the processor, then deliver terminal messages: they must
	// survive even after the flood.
	close(gate)
	pm.SendProgress(CreateErrorMsg("blocked", "url", errors.New("boom")))
	pm.SendProgress(CreateCompletedMsg("other", "url", "done"))

	pm.Complete(time.Second)

	pm.simple.mu.Lock()
	defer pm.simple.mu.Unlock()

	blocked := pm.simple.operations["blocked"]
	if blocked == nil || !blocked.failed() {
		t.Error("terminal error message was dropped")
	}
	other := pm.simple.operations["other"]
	if other == nil || !other.succeeded() {
		t.Error("terminal completed message was dropped")
	}
}

func TestProgressManager_CompleteDrainsBeforeSummary(t *testing.T) {
	pm := NewProgressManager(false)
	if err := pm.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	var buf bytes.Buffer
	pm.simple.mu.Lock()
	pm.simple.w = &buf
	pm.simple.mu.Unlock()

	for i := 0; i < 50; i++ {
		pm.SendProgress(CreateErrorMsg(fmt.Sprintf("repo%d", i), "url", errors.New("boom")))
	}
	pm.Complete(time.Second)

	out := buf.String()
	if !strings.Contains(out, "0 synced, ✗ 50 failed") {
		t.Errorf("summary must reflect every terminal message sent before Complete, got summary line in: %q", out)
	}
	// The summary must come after all per-repo lines.
	if idx := strings.Index(out, "synced"); strings.Count(out[idx:], "boom") != 0 {
		t.Error("progress lines were printed after the summary")
	}
}

func TestProgressManager_DoubleCompleteIsSafe(t *testing.T) {
	pm := NewProgressManager(false)
	if err := pm.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	pm.Complete(time.Second)
	pm.Complete(time.Second) // must not panic (previously: double close)
}

func TestProgressManager_SendAfterCompleteIsSafe(t *testing.T) {
	pm := NewProgressManager(false)
	if err := pm.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	pm.Complete(time.Second)

	// Must not panic (previously: send on closed channel).
	pm.SendProgress(CreateErrorMsg("late", "url", errors.New("boom")))
}

func TestProgressManager_SendBeforeStartIsSafe(t *testing.T) {
	pm := NewProgressManager(false)
	pm.SendProgress(CreateErrorMsg("early", "url", errors.New("boom")))
	pm.Complete(time.Second)
}

func TestProgressManager_RestartAfterComplete(t *testing.T) {
	pm := NewProgressManager(false)
	if err := pm.Start(); err != nil {
		t.Fatalf("first Start failed: %v", err)
	}
	pm.SendProgress(CreateCompletedMsg("first", "url", "done"))
	pm.Complete(time.Second)

	// A second run on the same manager (e.g. a second Sync) must work.
	if err := pm.Start(); err != nil {
		t.Fatalf("second Start failed: %v", err)
	}

	var buf bytes.Buffer
	pm.simple.mu.Lock()
	pm.simple.w = &buf
	pm.simple.mu.Unlock()

	pm.SendProgress(CreateErrorMsg("second", "url", errors.New("boom")))
	pm.Complete(time.Second)

	out := buf.String()
	if !strings.Contains(out, "0 synced, ✗ 1 failed") {
		t.Errorf("restarted manager must track a fresh run, got: %q", out)
	}
	if strings.Contains(out, "first") {
		t.Errorf("restarted manager must not carry over previous operations, got: %q", out)
	}
}

func TestProgressManager_StartIdempotentWhileRunning(t *testing.T) {
	pm := NewProgressManager(false)
	if err := pm.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	simple := pm.simple
	if err := pm.Start(); err != nil {
		t.Fatalf("second Start failed: %v", err)
	}
	if pm.simple != simple {
		t.Error("Start on a running manager must not reset state")
	}
	pm.Complete(time.Second)
}

func TestProgressManager_InteractiveCompleteDoesNotHangWithoutTTY(t *testing.T) {
	// Regression test: Complete previously waited on tea.Program.Wait, which
	// blocks forever when the program failed to start (no TTY, as in tests
	// and CI). Complete must return regardless of the program's fate.
	pm := NewProgressManager(true)
	if err := pm.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	pm.SendProgress(CreateErrorMsg("repo", "url", errors.New("boom")))

	done := make(chan struct{})
	go func() {
		defer close(done)
		pm.Complete(time.Second)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("interactive Complete hung without a TTY")
	}
}

func TestCreateErrorMsg(t *testing.T) {
	err := errors.New("boom")
	msg := CreateErrorMsg("repo", "url", err)

	if msg.Phase != types.PhaseFailed {
		t.Errorf("expected failed phase, got %s", msg.Phase)
	}
	if msg.Error != err {
		t.Error("expected error to be carried")
	}
	if !msg.IsComplete() {
		t.Error("error message must be terminal")
	}
	if msg.CompletedAt == nil {
		t.Error("expected CompletedAt to be set")
	}
}
