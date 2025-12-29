package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tierone/harbormaster/pkg/types"
)

// operationState tracks the state of a single operation.
type operationState struct {
	repoName  string
	phase     types.ProgressPhase
	percent   float64
	message   string
	err       error
	startedAt time.Time
	endedAt   *time.Time
}

func (o *operationState) isComplete() bool {
	return o.phase == types.PhaseComplete || o.phase == types.PhaseFailed
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
	progress   progress.Model
	width      int
	quitting   bool
	done       bool
}

// NewModel creates a new UI model.
func NewModel() Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = SpinnerStyle

	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(40),
		progress.WithoutPercentage(),
	)

	return Model{
		operations: make(map[string]*operationState),
		order:      []string{},
		spinner:    s,
		progress:   p,
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
		m.progress.Width = msg.Width - 50
		if m.progress.Width < 20 {
			m.progress.Width = 20
		}

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
	op.percent = msg.Percent
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
		if op.err != nil {
			b.WriteString(ErrorStyle.Render(op.err.Error()))
		} else {
			b.WriteString(SuccessStyle.Render(op.message))
		}
		// Duration
		b.WriteString(" ")
		b.WriteString(MutedStyle.Render(fmt.Sprintf("(%s)", op.duration().Round(time.Millisecond))))
	} else {
		// Show progress bar or phase
		if op.percent > 0 {
			b.WriteString(m.progress.ViewAs(op.percent / 100))
		} else {
			phase := PhaseColor(string(op.phase)).Render(string(op.phase))
			b.WriteString(phase)
			if op.message != "" {
				b.WriteString(" ")
				b.WriteString(MutedStyle.Render(op.message))
			}
		}
	}

	return b.String()
}

func (m *Model) getSymbol(op *operationState) string {
	if op.isComplete() {
		if op.err != nil {
			return SymbolError
		}
		return SymbolSuccess
	}
	return m.spinner.View()
}

func (m *Model) renderSummary() string {
	var success, failed int
	for _, op := range m.operations {
		if op.err != nil {
			failed++
		} else if op.phase == types.PhaseComplete {
			success++
		}
	}

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

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
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

// Simple progress output for non-interactive mode.
type SimpleOutput struct {
	operations map[string]*operationState
}

// NewSimpleOutput creates a simple non-interactive output.
func NewSimpleOutput() *SimpleOutput {
	return &SimpleOutput{
		operations: make(map[string]*operationState),
	}
}

// Update updates the output with a progress message.
func (s *SimpleOutput) Update(msg types.ProgressMsg) {
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

	// Print on phase change or completion
	if prevPhase != op.phase || op.isComplete() {
		s.print(op)
	}
}

func (s *SimpleOutput) print(op *operationState) {
	symbol := "●"
	style := lipgloss.NewStyle()

	switch op.phase {
	case types.PhaseComplete:
		symbol = "✓"
		style = SuccessStyle
	case types.PhaseFailed:
		symbol = "✗"
		style = ErrorStyle
	}

	msg := op.message
	if op.err != nil {
		msg = op.err.Error()
	}

	fmt.Printf("%s %s: %s %s\n",
		style.Render(symbol),
		op.repoName,
		string(op.phase),
		msg,
	)
}

// Complete prints the final summary.
func (s *SimpleOutput) Complete() {
	var success, failed int
	for _, op := range s.operations {
		if op.err != nil {
			failed++
		} else if op.phase == types.PhaseComplete {
			success++
		}
	}

	fmt.Println()
	if failed == 0 {
		fmt.Printf("✓ All %d repositories synced successfully\n", success)
	} else {
		fmt.Printf("✓ %d synced, ✗ %d failed\n", success, failed)
	}
}
