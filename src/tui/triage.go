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

	// Context style - dimmed for readability
	contextStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244")). // Gray
			Padding(0, 2)

	// Error line highlight in context
	errorLineStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")). // Red
			Bold(true)

	// Column widths
	confidenceWidth = 12
	recurrenceWidth = 12
	hashWidth       = 10
	snippetWidth    = 80
)

// TriageModel is the Bubble Tea model for the Triage Reporter TUI.
// It displays analyzed failure cards sorted by confidence score and recurrence.
type TriageModel struct {
	cards         []contracts.TriageCard // Pre-sorted list of triage cards
	viewport      viewport.Model         // Viewport for scrolling content
	ready         bool                   // Whether viewport is initialized
	cursor        int                    // Currently selected row (tracked separately from viewport scroll)
	expanded      map[int]bool           // Tracks which rows are expanded
	terminalWidth int                    // Current terminal width for dynamic sizing
}

// NewTriageModel creates a new TriageModel with the given sorted triage cards.
// Cards should be pre-sorted by ConfidenceScore (descending), then by RecurrenceCount.
func NewTriageModel(cards []contracts.TriageCard) TriageModel {
	return TriageModel{
		cards:    cards,
		ready:    false,
		cursor:   0,
		expanded: make(map[int]bool),
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
			m.terminalWidth = msg.Width
			m.viewport.SetContent(m.renderTable())
			m.ready = true
		} else {
			// Update viewport size and re-render if terminal resized
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - 7
			m.terminalWidth = msg.Width
			m.viewport.SetContent(m.renderTable()) // Re-render with new width
		}

	case tea.KeyMsg:
		switch msg.String() {
		// Exit commands
		case "q", "ctrl+c":
			return m, tea.Quit

		// Toggle expansion for current row
		case "enter", " ":
			m.expanded[m.cursor] = !m.expanded[m.cursor]
			m.viewport.SetContent(m.renderTable())
			return m, nil

		// Navigation with cursor tracking
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
			m.viewport.LineUp(1)
		case "down", "j":
			if m.cursor < len(m.cards)-1 {
				m.cursor++
			}
			m.viewport.LineDown(1)
		case "pgup", "b":
			m.cursor = max(0, m.cursor-10)
			m.viewport.ViewUp()
		case "pgdown", "f":
			m.cursor = min(len(m.cards)-1, m.cursor+10)
			m.viewport.ViewDown()
		case "home", "g":
			m.cursor = 0
			m.viewport.GotoTop()
		case "end", "G":
			m.cursor = len(m.cards) - 1
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
	helpText := "↑/↓ navigate • enter/space expand • pgup/pgdn page • g/G top/bottom • q quit"
	b.WriteString(lipgloss.NewStyle().Faint(true).Render(helpText))
	b.WriteString("\n")

	return b.String()
}

// renderTable generates the table content for the viewport
func (m TriageModel) renderTable() string {
	var b strings.Builder

	// Calculate dynamic snippet width based on terminal size
	// Account for: confidence (12) + recurrence (12) + hash (10) + spacing + cursor indicator
	fixedWidth := confidenceWidth + recurrenceWidth + hashWidth + 10 // +10 for spacing and cursor
	dynamicSnippetWidth := snippetWidth
	if m.terminalWidth > fixedWidth+snippetWidth {
		dynamicSnippetWidth = m.terminalWidth - fixedWidth - 5 // Extra margin
	}

	// Table rows
	for i, card := range m.cards {
		// Recurrence count - if metadata has it
		recurrenceCount := "1" // Default to 1 if not tracked yet
		if count, ok := card.Metadata["recurrence_count"]; ok {
			recurrenceCount = count
		}

		// Clean up normalized placeholders to show meaningful content
		snippet := cleanDisplayMessage(card.Message)

		// Truncate log snippet to prevent horizontal scrolling
		if len(snippet) > dynamicSnippetWidth {
			snippet = snippet[:dynamicSnippetWidth-3] + "..."
		}

		// Format row with cursor indicator
		row := fmt.Sprintf("%-*.2f %-*s %-*s %-*s",
			confidenceWidth, card.ConfidenceScore,
			recurrenceWidth, recurrenceCount,
			hashWidth, card.MessageHash[:8], // First 8 chars of hash
			dynamicSnippetWidth, snippet,
		)

		// Add cursor indicator and apply style
		if i == m.cursor {
			if m.expanded[i] {
				b.WriteString(selectedStyle.Render(row + " ▼"))
			} else {
				b.WriteString(selectedStyle.Render(row + " ▸"))
			}
		} else {
			b.WriteString(normalStyle.Render(row))
		}
		b.WriteString("\n")

		// If expanded, show context
		if m.expanded[i] {
			context := m.renderContext(card)
			b.WriteString(context)
			b.WriteString("\n")
		}
	}

	return b.String()
}

// renderContext renders the pre/post context for an expanded triage card
func (m TriageModel) renderContext(card contracts.TriageCard) string {
	var b strings.Builder

	// Job name header
	jobName := card.JobName
	if jobName == "" {
		jobName = "unknown"
	}
	b.WriteString(contextStyle.Render(fmt.Sprintf("  Job: %s", jobName)))
	b.WriteString("\n")

	// Pre-context (lines before the error)
	if card.PreContext != "" {
		b.WriteString(contextStyle.Render("  ─── Context (before) ───"))
		b.WriteString("\n")
		for _, line := range strings.Split(card.PreContext, "\n") {
			b.WriteString(contextStyle.Render(fmt.Sprintf("    %s", line)))
			b.WriteString("\n")
		}
	}

	// Error line (the actual detected failure)
	b.WriteString(errorLineStyle.Render("  ─── ERROR LINE ───"))
	b.WriteString("\n")
	b.WriteString(errorLineStyle.Render(fmt.Sprintf("    %s", card.Message)))
	b.WriteString("\n")

	// Post-context (lines after the error)
	if card.PostContext != "" {
		b.WriteString(contextStyle.Render("  ─── Context (after) ───"))
		b.WriteString("\n")
		for _, line := range strings.Split(card.PostContext, "\n") {
			b.WriteString(contextStyle.Render(fmt.Sprintf("    %s", line)))
			b.WriteString("\n")
		}
	}

	return b.String()
}

// cleanDisplayMessage removes normalized placeholders to show meaningful error content
func cleanDisplayMessage(msg string) string {
	cleaned := msg

	// Remove common normalized placeholders that clutter the display
	cleaned = strings.ReplaceAll(cleaned, "[TIMESTAMP] ", "")
	cleaned = strings.ReplaceAll(cleaned, "[TIMESTAMP]", "")
	cleaned = strings.ReplaceAll(cleaned, "[PID] ", "")
	cleaned = strings.ReplaceAll(cleaned, "[PID]", "")
	cleaned = strings.ReplaceAll(cleaned, "[UUID] ", "")
	cleaned = strings.ReplaceAll(cleaned, "[UUID]", "")
	cleaned = strings.ReplaceAll(cleaned, "[ADDR] ", "")
	cleaned = strings.ReplaceAll(cleaned, "[ADDR]", "")
	cleaned = strings.ReplaceAll(cleaned, "[LINE] ", "")
	cleaned = strings.ReplaceAll(cleaned, "[LINE]", "")

	// Trim leading/trailing whitespace after replacements
	cleaned = strings.TrimSpace(cleaned)

	return cleaned
}

// Helper functions for min/max
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
