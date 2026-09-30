package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rivo/uniseg"
	"github.com/tierone/harbormaster/pkg/types"
)

// operationState tracks the state of a single operation.
type operationState struct {
	repoName  string
	phase     types.ProgressPhase
	message   string
	err       error
	startedAt time.Time
	endedAt   *time.Time
}

func (o *operationState) isComplete() bool {
	return o.phase == types.PhaseComplete || o.phase == types.PhaseFailed
}

// failed reports whether the operation ended in failure. An operation counts
// as failed if it reached the failed phase or carries an error, regardless of
// which of the two was set.
func (o *operationState) failed() bool {
	return o.phase == types.PhaseFailed || o.err != nil
}

// succeeded reports whether the operation completed successfully.
func (o *operationState) succeeded() bool {
	return o.phase == types.PhaseComplete && o.err == nil
}

// summarizeOperations is the single counting predicate shared by the
// interactive summary (Model.renderSummary) and the simple output
// (SimpleOutput.Complete). A repository counts as a success only if it
// reached the complete phase without an error; everything else (failed
// phase, error, or never finished) counts as a failure.
func summarizeOperations(ops map[string]*operationState) (success, failed int) {
	for _, op := range ops {
		if op.succeeded() {
			success++
		} else {
			failed++
		}
	}
	return success, failed
}

func (o *operationState) duration() time.Duration {
	if o.endedAt != nil {
		return o.endedAt.Sub(o.startedAt)
	}
	return time.Since(o.startedAt)
}

// Model is the Bubbletea model for the progress UI.
type Model struct {
	operations map[string]*operationState
	order      []string // Maintains insertion order
	spinner    spinner.Model
	width      int
	quitting   bool
	done       bool
}

// NewModel creates a new UI model.
func NewModel() Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = SpinnerStyle

	return Model{
		operations: make(map[string]*operationState),
		order:      []string{},
		spinner:    s,
		width:      80,
	}
}

// ProgressMsg is sent to update operation progress.
type ProgressMsg types.ProgressMsg

// CompleteMsg signals that all operations are complete.
type CompleteMsg struct{}

// Init initializes the model.
func (m Model) Init() tea.Cmd {
	return m.spinner.Tick
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case ProgressMsg:
		m.updateOperation(msg)
		return m, nil

	case CompleteMsg:
		m.done = true
		return m, tea.Quit
	}

	return m, nil
}

func (m *Model) updateOperation(msg ProgressMsg) {
	op, exists := m.operations[msg.RepoName]
	if !exists {
		op = &operationState{
			repoName:  msg.RepoName,
			startedAt: msg.StartedAt,
		}
		m.operations[msg.RepoName] = op
		m.order = append(m.order, msg.RepoName)
	}

	op.phase = msg.Phase
	op.message = msg.Message
	op.err = msg.Error

	if msg.CompletedAt != nil {
		op.endedAt = msg.CompletedAt
	}
}

// View renders the UI.
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	// Header
	b.WriteString(HeaderStyle.Render("Harbormaster Sync"))
	b.WriteString("\n\n")

	// Operations
	for _, name := range m.order {
		op := m.operations[name]
		b.WriteString(m.renderOperation(op))
		b.WriteString("\n")
	}

	// Summary if done
	if m.done {
		b.WriteString("\n")
		b.WriteString(m.renderSummary())
	} else {
		// Help text
		b.WriteString("\n")
		b.WriteString(MutedStyle.Render("Press q to quit"))
	}

	return b.String()
}

func (m *Model) renderOperation(op *operationState) string {
	var b strings.Builder

	// Status symbol
	symbol := m.getSymbol(op)
	b.WriteString(symbol)
	b.WriteString(" ")

	// Repository name
	name := RepoNameStyle.Render(truncate(op.repoName, 28))
	b.WriteString(name)
	b.WriteString(" ")

	// Phase or progress bar
	if op.isComplete() {
		switch {
		case op.err != nil:
			b.WriteString(ErrorStyle.Render(op.err.Error()))
		case op.failed():
			msg := op.message
			if msg == "" {
				msg = "failed"
			}
			b.WriteString(ErrorStyle.Render(msg))
		default:
			b.WriteString(SuccessStyle.Render(op.message))
		}
		// Duration
		b.WriteString(" ")
		b.WriteString(MutedStyle.Render(fmt.Sprintf("(%s)", op.duration().Round(time.Millisecond))))
	} else {
		// Running: show the phase and the latest activity message. There is
		// deliberately no progress bar: for git clones the percentage is
		// parsed from git's stderr and resets per phase and per submodule,
		// so a bar claims precision that does not exist. The message (e.g.
		// "Cloning into 'external/foo'...") is the truthful signal.
		phase := PhaseColor(string(op.phase)).Render(string(op.phase))
		b.WriteString(phase)
		if op.message != "" {
			// Measure the already-rendered prefix (symbol, padded name,
			// phase) rather than guessing a constant, and omit the message
			// entirely when not even a truncated fragment fits.
			remaining := m.width - lipgloss.Width(b.String()) - 1
			if remaining >= 2 {
				b.WriteString(" ")
				b.WriteString(MutedStyle.Render(truncate(op.message, remaining)))
			}
		}
	}

	return b.String()
}

func (m *Model) getSymbol(op *operationState) string {
	if op.isComplete() {
		if op.failed() {
			return SymbolError
		}
		return SymbolSuccess
	}
	return m.spinner.View()
}

func (m *Model) renderSummary() string {
	success, failed := summarizeOperations(m.operations)

	var b strings.Builder
	b.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	if failed == 0 {
		b.WriteString(SummarySuccessStyle.Render(
			fmt.Sprintf("✓ All %d repositories synced successfully", success),
		))
	} else {
		b.WriteString(SummarySuccessStyle.Render(fmt.Sprintf("✓ %d synced", success)))
		b.WriteString("  ")
		b.WriteString(SummaryErrorStyle.Render(fmt.Sprintf("✗ %d failed", failed)))
	}

	return b.String()
}

// truncate shortens s to at most maxCells display cells, appending an
// ellipsis when truncating. It iterates grapheme clusters, so it never
// splits a UTF-8 sequence, a ZWJ emoji sequence, or a combining-mark
// cluster, and wide characters count as two cells — non-ASCII repository
// and submodule paths render correctly.
func truncate(s string, maxCells int) string {
	if maxCells <= 0 {
		return ""
	}
	if uniseg.StringWidth(s) <= maxCells {
		return s
	}
	if maxCells == 1 {
		return "…"
	}
	var b strings.Builder
	w := 0
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		cw := g.Width()
		if w+cw > maxCells-1 { // reserve one cell for the ellipsis
			break
		}
		b.WriteString(g.Str())
		w += cw
	}
	return b.String() + "…"
}

// SendProgress sends a progress message to the program.
func SendProgress(p *tea.Program, msg types.ProgressMsg) {
	if p != nil {
		p.Send(ProgressMsg(msg))
	}
}

// SendComplete signals completion to the program.
func SendComplete(p *tea.Program) {
	if p != nil {
		p.Send(CompleteMsg{})
	}
}

// Run starts the Bubbletea program and returns the final model.
func Run(m Model) (Model, error) {
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return m, err
	}
	return finalModel.(Model), nil
}

// SimpleOutput is a simple progress output for non-interactive mode.
// It is safe for concurrent use.
type SimpleOutput struct {
	mu         sync.Mutex
	w          io.Writer
	operations map[string]*operationState
}

// NewSimpleOutput creates a simple non-interactive output writing to stdout.
func NewSimpleOutput() *SimpleOutput {
	return NewSimpleOutputTo(os.Stdout)
}

// NewSimpleOutputTo creates a simple non-interactive output writing to w.
func NewSimpleOutputTo(w io.Writer) *SimpleOutput {
	return &SimpleOutput{
		w:          w,
		operations: make(map[string]*operationState),
	}
}

// Update updates the output with a progress message.
func (s *SimpleOutput) Update(msg types.ProgressMsg) {
	s.mu.Lock()
	defer s.mu.Unlock()

	op, exists := s.operations[msg.RepoName]
	if !exists {
		op = &operationState{
			repoName:  msg.RepoName,
			startedAt: msg.StartedAt,
		}
		s.operations[msg.RepoName] = op
	}

	prevPhase := op.phase
	op.phase = msg.Phase
	op.message = msg.Message
	op.err = msg.Error
	if msg.CompletedAt != nil {
		op.endedAt = msg.CompletedAt
	}

	// Print on phase change or completion
	if prevPhase != op.phase || op.isComplete() {
		s.print(op)
	}
}

// print writes a single operation line. Callers must hold s.mu.
func (s *SimpleOutput) print(op *operationState) {
	symbol := "●"
	style := lipgloss.NewStyle()

	switch {
	case op.failed():
		symbol = "✗"
		style = ErrorStyle
	case op.phase == types.PhaseComplete:
		symbol = "✓"
		style = SuccessStyle
	}

	msg := op.message
	if op.err != nil {
		msg = op.err.Error()
	}

	_, _ = fmt.Fprintf(s.w, "%s %s: %s %s\n",
		style.Render(symbol),
		op.repoName,
		string(op.phase),
		msg,
	)
}

// Complete prints the final summary.
func (s *SimpleOutput) Complete() {
	s.mu.Lock()
	defer s.mu.Unlock()

	success, failed := summarizeOperations(s.operations)

	_, _ = fmt.Fprintln(s.w)
	if failed == 0 {
		_, _ = fmt.Fprintf(s.w, "✓ All %d repositories synced successfully\n", success)
	} else {
		_, _ = fmt.Fprintf(s.w, "✓ %d synced, ✗ %d failed\n", success, failed)
	}
}
