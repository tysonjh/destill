// Package tui provides the Terminal User Interface for the Destill log triage tool.
// This TUI serves as a Triage Reporter to validate and display analyzed failure cards.
package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"destill-agent/src/contracts"
)

// Styles for the TUI
var (
	// Header style - bold and visually distinct
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("12")). // Bright blue
			Background(lipgloss.Color("236")). // Dark gray
			Padding(0, 1)

	// Selected row style - reversed colors
	selectedStyle = lipgloss.NewStyle().
			Reverse(true).
			Padding(0, 1)

	// Normal row style
	normalStyle = lipgloss.NewStyle().
			Padding(0, 1)

	// Column widths
	confidenceWidth = 12
	recurrenceWidth = 12
	hashWidth       = 10
	snippetWidth    = 80
)

// TriageModel is the Bubble Tea model for the Triage Reporter TUI.
// It displays analyzed failure cards sorted by confidence score and recurrence.
type TriageModel struct {
	cards  []contracts.TriageCard // Pre-sorted list of triage cards
	cursor int                     // Currently selected row index
}

// NewTriageModel creates a new TriageModel with the given sorted triage cards.
// Cards should be pre-sorted by ConfidenceScore (descending), then by RecurrenceCount.
func NewTriageModel(cards []contracts.TriageCard) TriageModel {
	return TriageModel{
		cards:  cards,
		cursor: 0,
	}
}

// Init initializes the model. Required by tea.Model interface.
func (m TriageModel) Init() tea.Cmd {
	return nil
}

// Update handles messages and updates the model state.
// Implements navigation and exit commands.
func (m TriageModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		// Exit commands
		case "q", "ctrl+c":
			return m, tea.Quit

		// Navigation - up
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		// Navigation - down
		case "down", "j":
			if m.cursor < len(m.cards)-1 {
				m.cursor++
			}
		}
	}

	return m, nil
}

// View renders the TUI display.
// Displays a table of triage cards with intelligence metrics.
func (m TriageModel) View() string {
	// Handle empty input
	if len(m.cards) == 0 {
		return "\nNo failures detected or analyzed.\n\n"
	}

	var b strings.Builder

	// Title
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Destill - CI/CD Failure Triage Report"))
	b.WriteString("\n\n")

	// Table header
	header := fmt.Sprintf("%-*s %-*s %-*s %-*s",
		confidenceWidth, "Confidence",
		recurrenceWidth, "Recurrence",
		hashWidth, "Hash",
		snippetWidth, "Log Snippet",
	)
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n")

	// Table rows
	for i, card := range m.cards {
		// Recurrence count - if metadata has it
		recurrenceCount := "1" // Default to 1 if not tracked yet
		if count, ok := card.Metadata["recurrence_count"]; ok {
			recurrenceCount = count
		}

		// Truncate log snippet to prevent horizontal scrolling
		snippet := card.Message
		if len(snippet) > snippetWidth {
			snippet = snippet[:snippetWidth-3] + "..."
		}

		// Format row
		row := fmt.Sprintf("%-*.2f %-*s %-*s %-*s",
			confidenceWidth, card.ConfidenceScore,
			recurrenceWidth, recurrenceCount,
			hashWidth, card.MessageHash[:8], // First 8 chars of hash
			snippetWidth, snippet,
		)

		// Apply style based on selection
		if i == m.cursor {
			b.WriteString(selectedStyle.Render(row))
		} else {
			b.WriteString(normalStyle.Render(row))
		}
		b.WriteString("\n")
	}

	// Help text
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("Navigation: ↑/k up • ↓/j down • q/ctrl+c quit"))
	b.WriteString("\n")

	return b.String()
}
