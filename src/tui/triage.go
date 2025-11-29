// Package tui provides the Terminal User Interface for the Destill log triage tool.
// This TUI serves as a Triage Reporter to validate and display analyzed failure cards.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"destill-agent/src/contracts"
)

// Styles for the TUI
var (
	// Header style - bold and visually distinct
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("12")).  // Bright blue
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
	cards    []contracts.TriageCard // Pre-sorted list of triage cards
	viewport viewport.Model         // Viewport for scrolling content
	ready    bool                   // Whether viewport is initialized
}

// NewTriageModel creates a new TriageModel with the given sorted triage cards.
// Cards should be pre-sorted by ConfidenceScore (descending), then by RecurrenceCount.
func NewTriageModel(cards []contracts.TriageCard) TriageModel {
	return TriageModel{
		cards: cards,
		ready: false,
	}
}

// Init initializes the model. Required by tea.Model interface.
func (m TriageModel) Init() tea.Cmd {
	return nil
}

// Update handles messages and updates the model state.
// Implements navigation and exit commands.
func (m TriageModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Initialize viewport on first window size message
		if !m.ready {
			// Reserve space for title, header, and help text
			headerHeight := 5 // Title + spacing + header + spacing
			footerHeight := 2 // Help text + spacing
			m.viewport = viewport.New(msg.Width, msg.Height-headerHeight-footerHeight)
			m.viewport.SetContent(m.renderTable())
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - 7
		}

	case tea.KeyMsg:
		switch msg.String() {
		// Exit commands
		case "q", "ctrl+c":
			return m, tea.Quit

		// Navigation handled by viewport
		case "up", "k":
			m.viewport.LineUp(1)
		case "down", "j":
			m.viewport.LineDown(1)
		case "pgup", "b":
			m.viewport.ViewUp()
		case "pgdown", "f", " ":
			m.viewport.ViewDown()
		case "home", "g":
			m.viewport.GotoTop()
		case "end", "G":
			m.viewport.GotoBottom()
		}
	}

	// Update viewport
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// View renders the TUI display.
// Displays a table of triage cards with intelligence metrics.
func (m TriageModel) View() string {
	if !m.ready {
		return "\nInitializing..."
	}

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

	// Viewport with table content
	b.WriteString(m.viewport.View())

	// Help text
	b.WriteString("\n")
	helpText := "Navigation: ↑/k up • ↓/j down • pgup/pgdn page • g/G top/bottom • q/ctrl+c quit"
	b.WriteString(lipgloss.NewStyle().Faint(true).Render(helpText))
	b.WriteString("\n")

	return b.String()
}

// renderTable generates the table content for the viewport
func (m TriageModel) renderTable() string {
	var b strings.Builder

	// Table rows
	for _, card := range m.cards {
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

		b.WriteString(normalStyle.Render(row))
		b.WriteString("\n")
	}

	return b.String()
}
