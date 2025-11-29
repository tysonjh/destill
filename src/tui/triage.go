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
			Foreground(lipgloss.Color("12")).  // Bright blue
			Background(lipgloss.Color("236")). // Dark gray
			Padding(0, 1)

	// Selected row style - subtle with left indicator
	selectedStyle = lipgloss.NewStyle().
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
	cursor         int                    // Currently selected row
	listScroll     int                    // Scroll offset for list view
	detailScroll   int                    // Scroll offset for detail view
	terminalWidth  int                    // Terminal width for dynamic sizing
	terminalHeight int                    // Terminal height for split calculation
}

// NewTriageModel creates a new TriageModel with the given sorted triage cards.
// Cards should be pre-sorted by ConfidenceScore (descending), then by RecurrenceCount.
func NewTriageModel(cards []contracts.TriageCard) TriageModel {
	return TriageModel{
		cards:        cards,
		cursor:       0,
		listScroll:   0,
		detailScroll: 0,
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
	case tea.WindowSizeMsg:
		m.terminalWidth = msg.Width
		m.terminalHeight = msg.Height

	case tea.KeyMsg:
		// Calculate list height for smart scrolling
		listHeight := (m.terminalHeight - 8) / 4
		if listHeight < 2 {
			listHeight = 2
		}

		switch msg.String() {
		// Exit commands
		case "q", "ctrl+c":
			return m, tea.Quit

		// Navigation - update cursor and auto-scroll list
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				// Auto-scroll list if cursor goes above visible area
				if m.cursor < m.listScroll {
					m.listScroll = m.cursor
				}
				m.detailScroll = 0 // Reset detail scroll when changing items
			}
		case "down", "j":
			if m.cursor < len(m.cards)-1 {
				m.cursor++
				// Auto-scroll list if cursor goes below visible area
				if m.cursor >= m.listScroll+listHeight {
					m.listScroll = m.cursor - listHeight + 1
				}
				m.detailScroll = 0 // Reset detail scroll when changing items
			}
		case "pgup", "b":
			m.cursor = max(0, m.cursor-10)
			m.listScroll = max(0, m.cursor-listHeight/2)
			m.detailScroll = 0
		case "pgdown", "f", " ":
			m.cursor = min(len(m.cards)-1, m.cursor+10)
			m.listScroll = max(0, min(m.cursor-listHeight/2, len(m.cards)-listHeight))
			m.detailScroll = 0
		case "home", "g":
			m.cursor = 0
			m.listScroll = 0
			m.detailScroll = 0
		case "end", "G":
			m.cursor = len(m.cards) - 1
			m.listScroll = max(0, len(m.cards)-listHeight)
			m.detailScroll = 0

		// Scroll detail view independently
		case "d":
			m.detailScroll++
		case "u":
			if m.detailScroll > 0 {
				m.detailScroll--
			}
		}
	}

	return m, nil
} // View renders the TUI display with split-view layout.
// Top section shows scrollable failure list, bottom shows detail for selected item.
func (m TriageModel) View() string {
	if m.terminalHeight == 0 {
		return "Initializing..."
	}

	if len(m.cards) == 0 {
		return "No failures detected or analyzed.\n"
	}

	var b strings.Builder

	// Calculate heights
	// UI overhead: title (1) + header (1) + divider (1) + help (1) = 4 lines
	availableHeight := m.terminalHeight - 4
	if availableHeight < 8 {
		availableHeight = 8
	}
	listHeight := availableHeight / 4
	if listHeight < 2 {
		listHeight = 2
	}
	detailHeight := availableHeight - listHeight

	// Title
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Destill - CI/CD Failure Triage Report"))
	b.WriteString("\n")

	// Header for list
	header := fmt.Sprintf("%-*s %-*s %-*s %s",
		confidenceWidth, "Confidence",
		recurrenceWidth, "Recurrence",
		hashWidth, "Hash",
		"Error Message",
	)
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n")

	// Render visible list items
	listLines := m.renderList()
	visibleStart := m.listScroll
	visibleEnd := min(visibleStart+listHeight, len(listLines))
	for i := visibleStart; i < visibleEnd; i++ {
		b.WriteString(listLines[i])
		b.WriteString("\n")
	}
	// Pad if needed
	for i := visibleEnd - visibleStart; i < listHeight; i++ {
		b.WriteString("\n")
	}

	// Divider
	divider := strings.Repeat("─", m.terminalWidth)
	b.WriteString(dividerStyle.Render(divider))
	b.WriteString("\n")

	// Render visible detail lines
	detailLines := m.renderDetail()
	detailStart := m.detailScroll
	detailEnd := min(detailStart+detailHeight, len(detailLines))
	for i := detailStart; i < detailEnd; i++ {
		b.WriteString(detailLines[i])
		b.WriteString("\n")
	}
	// Pad if needed
	for i := detailEnd - detailStart; i < detailHeight; i++ {
		b.WriteString("\n")
	}

	// Help text
	helpText := "↑/↓ navigate list • d/u scroll detail • g/G top/bottom • q quit"
	b.WriteString(lipgloss.NewStyle().Faint(true).Render(helpText))

	return b.String()
}

// renderList generates the failure list lines
func (m TriageModel) renderList() []string {
	var lines []string

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

		// Highlight selected row with subtle cursor indicator
		if i == m.cursor {
			cursor := lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Render("► ")
			lines = append(lines, cursor+selectedStyle.Render(row))
		} else {
			lines = append(lines, "  "+normalStyle.Render(row))
		}
	}

	return lines
}

// renderDetail generates the detail view lines for the selected failure
func (m TriageModel) renderDetail() []string {
	if m.cursor >= len(m.cards) {
		return []string{"No failure selected"}
	}

	card := m.cards[m.cursor]
	var lines []string

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
	lines = append(lines, detailHeaderStyle.Render(headerText))
	lines = append(lines, "")

	// Pre-context (lines before the error)
	if card.PreContext != "" {
		lines = append(lines, contextStyle.Render("─── Context (before) ───"))
		for _, line := range strings.Split(card.PreContext, "\n") {
			if strings.TrimSpace(line) != "" {
				lines = append(lines, contextStyle.Render(line))
			}
		}
		lines = append(lines, "")
	}

	// Error line (the actual detected failure)
	lines = append(lines, errorLineStyle.Render("─── ERROR LINE ───"))
	errorMsg := cleanDisplayMessage(card.Message)
	lines = append(lines, errorLineStyle.Render(errorMsg))
	lines = append(lines, "")

	// Post-context (lines after the error)
	if card.PostContext != "" {
		lines = append(lines, contextStyle.Render("─── Context (after) ───"))
		for _, line := range strings.Split(card.PostContext, "\n") {
			if strings.TrimSpace(line) != "" {
				lines = append(lines, contextStyle.Render(line))
			}
		}
	}

	return lines
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
