package ui

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	// Colors
	colorPrimary   = lipgloss.Color("39")  // Blue
	colorSuccess   = lipgloss.Color("82")  // Green
	colorWarning   = lipgloss.Color("214") // Orange
	colorError     = lipgloss.Color("196") // Red
	colorMuted     = lipgloss.Color("241") // Gray
	colorHighlight = lipgloss.Color("213") // Pink

	// Text styles
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary)

	SubtitleStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	SuccessStyle = lipgloss.NewStyle().
			Foreground(colorSuccess)

	WarningStyle = lipgloss.NewStyle().
			Foreground(colorWarning)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(colorError).
			Bold(true)

	MutedStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	HighlightStyle = lipgloss.NewStyle().
			Foreground(colorHighlight)

	// Status indicators
	SpinnerStyle = lipgloss.NewStyle().
			Foreground(colorPrimary)

	ProgressStyle = lipgloss.NewStyle().
			Foreground(colorPrimary)

	// Repository name style
	RepoNameStyle = lipgloss.NewStyle().
			Bold(true).
			Width(30)

	// Phase styles
	PhaseStyle = lipgloss.NewStyle().
			Width(15)

	// Symbols
	SymbolSuccess = SuccessStyle.Render("✓")
	SymbolError   = ErrorStyle.Render("✗")
	SymbolPending = MutedStyle.Render("○")
	SymbolRunning = SpinnerStyle.Render("●")

	// Box styles
	BoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorMuted).
			Padding(0, 1)

	// Header style
	HeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			MarginBottom(1)

	// Summary styles
	SummarySuccessStyle = lipgloss.NewStyle().
				Foreground(colorSuccess).
				Bold(true)

	SummaryErrorStyle = lipgloss.NewStyle().
				Foreground(colorError).
				Bold(true)
)

// StatusSymbol returns the appropriate symbol for a status.
func StatusSymbol(success bool, running bool, pending bool) string {
	if running {
		return SymbolRunning
	}
	if pending {
		return SymbolPending
	}
	if success {
		return SymbolSuccess
	}
	return SymbolError
}

// PhaseColor returns the appropriate style for a phase.
func PhaseColor(phase string) lipgloss.Style {
	switch phase {
	case "complete":
		return SuccessStyle
	case "failed":
		return ErrorStyle
	case "fetching", "checkout":
		return HighlightStyle
	default:
		return MutedStyle
	}
}
