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

	// Selected row style - bold with bright background for visibility
	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")). // White text
			Background(lipgloss.Color("33")). // Bright blue background
			Padding(0, 1)

	// Normal row style
	normalStyle = lipgloss.NewStyle().
			Padding(0, 1)

	// Border/divider style for split view
	dividerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")). // Dark gray
			Bold(true)

	// Context style - dimmed for readability
	contextStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244")). // Gray
			Padding(0, 1)

	// Error line highlight in context
	errorLineStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")). // Red
			Bold(true).
			Padding(0, 1)

	// Detail panel header style
	detailHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("10")). // Bright green
				Padding(0, 1)

	// Column widths
	confidenceWidth = 12
	recurrenceWidth = 12
	hashWidth       = 10
	snippetWidth    = 80
)

// TriageModel is the Bubble Tea model for the Triage Reporter TUI.
// It displays analyzed failure cards in a split-view layout:
// - Top 1/4: Scrollable list of failures
// - Bottom 3/4: Detail view with full context for selected failure
type TriageModel struct {
	cards          []contracts.TriageCard // Pre-sorted list of triage cards
	listViewport   viewport.Model         // Top viewport for failure list
	detailViewport viewport.Model         // Bottom viewport for detail view
	ready          bool                   // Whether viewports are initialized
	cursor         int                    // Currently selected row
	terminalWidth  int                    // Terminal width for dynamic sizing
	terminalHeight int                    // Terminal height for split calculation
}

// NewTriageModel creates a new TriageModel with the given sorted triage cards.
// Cards should be pre-sorted by ConfidenceScore (descending), then by RecurrenceCount.
func NewTriageModel(cards []contracts.TriageCard) TriageModel {
	return TriageModel{
		cards:  cards,
		ready:  false,
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
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.terminalWidth = msg.Width
		m.terminalHeight = msg.Height

		// Calculate split: 1/4 for list, 3/4 for detail
		// Reserve space for: title (1) + header (2) + divider (1) + help (2) = 6 lines
		availableHeight := msg.Height - 6
		listHeight := availableHeight / 4
		detailHeight := availableHeight - listHeight

		if !m.ready {
			// Initialize both viewports
			m.listViewport = viewport.New(msg.Width, listHeight)
			m.detailViewport = viewport.New(msg.Width, detailHeight)
			m.listViewport.SetContent(m.renderList())
			m.detailViewport.SetContent(m.renderDetail())
			m.ready = true
		} else {
			// Update viewport sizes and re-render on resize
			m.listViewport.Width = msg.Width
			m.listViewport.Height = listHeight
			m.detailViewport.Width = msg.Width
			m.detailViewport.Height = detailHeight
			m.listViewport.SetContent(m.renderList())
			m.detailViewport.SetContent(m.renderDetail())
		}

	case tea.KeyMsg:
		switch msg.String() {
		// Exit commands
		case "q", "ctrl+c":
			return m, tea.Quit

		// Navigation updates both list highlight and detail view
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.listViewport.SetContent(m.renderList())
				m.detailViewport.SetContent(m.renderDetail())
				m.detailViewport.GotoTop() // Reset detail scroll on selection change
			}
			m.listViewport.LineUp(1)
		case "down", "j":
			if m.cursor < len(m.cards)-1 {
				m.cursor++
				m.listViewport.SetContent(m.renderList())
				m.detailViewport.SetContent(m.renderDetail())
				m.detailViewport.GotoTop() // Reset detail scroll on selection change
			}
			m.listViewport.LineDown(1)
		case "pgup", "b":
			m.cursor = max(0, m.cursor-10)
			m.listViewport.SetContent(m.renderList())
			m.detailViewport.SetContent(m.renderDetail())
			m.detailViewport.GotoTop()
			m.listViewport.ViewUp()
		case "pgdown", "f", " ":
			m.cursor = min(len(m.cards)-1, m.cursor+10)
			m.listViewport.SetContent(m.renderList())
			m.detailViewport.SetContent(m.renderDetail())
			m.detailViewport.GotoTop()
			m.listViewport.ViewDown()
		case "home", "g":
			m.cursor = 0
			m.listViewport.SetContent(m.renderList())
			m.detailViewport.SetContent(m.renderDetail())
			m.detailViewport.GotoTop()
			m.listViewport.GotoTop()
		case "end", "G":
			m.cursor = len(m.cards) - 1
			m.listViewport.SetContent(m.renderList())
			m.detailViewport.SetContent(m.renderDetail())
			m.detailViewport.GotoTop()
			m.listViewport.GotoBottom()

		// Scroll detail view independently
		case "d":
			m.detailViewport.LineDown(1)
		case "u":
			m.detailViewport.LineUp(1)
		}
	}

	// Update list viewport (detail viewport is updated on navigation)
	m.listViewport, cmd = m.listViewport.Update(msg)
	return m, cmd
}

// View renders the TUI display with split-view layout.
// Top section shows scrollable failure list, bottom shows detail for selected item.
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

	// Top section: Failure list with header
	header := fmt.Sprintf("%-*s %-*s %-*s %-*s",
		confidenceWidth, "Confidence",
		recurrenceWidth, "Recurrence",
		hashWidth, "Hash",
		m.terminalWidth-confidenceWidth-recurrenceWidth-hashWidth-10, "Error Message",
	)
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n")
	b.WriteString(m.listViewport.View())

	// Divider between list and detail
	b.WriteString("\n")
	divider := strings.Repeat("─", m.terminalWidth)
	b.WriteString(dividerStyle.Render(divider))
	b.WriteString("\n")

	// Bottom section: Detail view
	b.WriteString(m.detailViewport.View())

	// Help text
	b.WriteString("\n")
	helpText := "↑/↓ navigate list • d/u scroll detail • g/G top/bottom • q quit"
	b.WriteString(lipgloss.NewStyle().Faint(true).Render(helpText))
	b.WriteString("\n")

	return b.String()
}

// renderList generates the failure list content for the top viewport
func (m TriageModel) renderList() string {
	var b strings.Builder

	// Calculate dynamic snippet width
	fixedWidth := confidenceWidth + recurrenceWidth + hashWidth + 10
	dynamicSnippetWidth := m.terminalWidth - fixedWidth - 5
	if dynamicSnippetWidth < 40 {
		dynamicSnippetWidth = 40 // Minimum width
	}

	// Render each failure as a single line
	for i, card := range m.cards {
		recurrenceCount := "1"
		if count, ok := card.Metadata["recurrence_count"]; ok {
			recurrenceCount = count
		}

		// Clean and truncate snippet
		snippet := cleanDisplayMessage(card.Message)
		if len(snippet) > dynamicSnippetWidth {
			snippet = snippet[:dynamicSnippetWidth-3] + "..."
		}

		// Format row
		row := fmt.Sprintf("%-*.2f %-*s %-*s %s",
			confidenceWidth, card.ConfidenceScore,
			recurrenceWidth, recurrenceCount,
			hashWidth, card.MessageHash[:8],
			snippet,
		)

		// Highlight selected row with cursor indicator
		if i == m.cursor {
			b.WriteString(selectedStyle.Render("▸ " + row))
		} else {
			b.WriteString(normalStyle.Render("  " + row))
		}
		b.WriteString("\n")
	}

	return b.String()
}

// renderDetail generates the detail view content for the bottom viewport
func (m TriageModel) renderDetail() string {
	if m.cursor >= len(m.cards) {
		return "No failure selected"
	}

	card := m.cards[m.cursor]
	var b strings.Builder

	// Detail header with job info
	jobName := card.JobName
	if jobName == "" {
		jobName = "unknown"
	}
	recurrenceCount := "1"
	if count, ok := card.Metadata["recurrence_count"]; ok {
		recurrenceCount = count
	}

	headerText := fmt.Sprintf("Job: %s │ Confidence: %.2f │ Occurrences: %s │ Hash: %s",
		jobName, card.ConfidenceScore, recurrenceCount, card.MessageHash[:8])
	b.WriteString(detailHeaderStyle.Render(headerText))
	b.WriteString("\n\n")

	// Pre-context (lines before the error)
	if card.PreContext != "" {
		b.WriteString(contextStyle.Render("─── Context (before) ───"))
		b.WriteString("\n")
		for _, line := range strings.Split(card.PreContext, "\n") {
			if strings.TrimSpace(line) != "" {
				b.WriteString(contextStyle.Render(line))
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}

	// Error line (the actual detected failure)
	b.WriteString(errorLineStyle.Render("─── ERROR LINE ───"))
	b.WriteString("\n")
	errorMsg := cleanDisplayMessage(card.Message)
	b.WriteString(errorLineStyle.Render(errorMsg))
	b.WriteString("\n\n")

	// Post-context (lines after the error)
	if card.PostContext != "" {
		b.WriteString(contextStyle.Render("─── Context (after) ───"))
		b.WriteString("\n")
		for _, line := range strings.Split(card.PostContext, "\n") {
			if strings.TrimSpace(line) != "" {
				b.WriteString(contextStyle.Render(line))
				b.WriteString("\n")
			}
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
