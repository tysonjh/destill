package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// View renders the complete TUI layout
func (m MainModel) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	// Render header
	header := m.header.Render(m.width)
	headerHeight := lipgloss.Height(header)

	// Calculate available height for panels
	// Account for: header + help line + list panel header row
	availableHeight := m.height - headerHeight - 1 - 1

	// Two-panel layout: Triage List (40%) | Context Detail (60%)
	// The lipgloss JoinHorizontal function handles the space.
	contentWidth := m.width
	leftPanelWidth := int(float64(contentWidth) * 0.4)
	rightPanelWidth := contentWidth - leftPanelWidth

	// Render panels
	leftPanel := m.renderListPanel(leftPanelWidth, availableHeight)
	rightPanel := m.renderDetailPanel(rightPanelWidth, availableHeight)

	// Combine panels horizontally
	mainContent := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)

	// Build help text
	help := m.renderHelpText()

	return lipgloss.JoinVertical(lipgloss.Left, header, mainContent, help)
}

// renderHelpText renders context-aware help text at the bottom
func (m MainModel) renderHelpText() string {
	keyStyle := lipgloss.NewStyle().Foreground(m.styles.PrimaryBlue).Bold(true)
	sepStyle := lipgloss.NewStyle().Foreground(m.styles.TextSecondary)

	var helpText string
	if m.detailFocused {
		helpText = fmt.Sprintf("%s: Scroll %s %s: Back %s %s: Quit",
			keyStyle.Render("j/k"), sepStyle.Render("•"),
			keyStyle.Render("Esc"), sepStyle.Render("•"),
			keyStyle.Render("q"))
	} else {
		helpText = fmt.Sprintf("%s: Nav %s %s: View %s %s: Job %s %s: Search %s %s: Reset %s %s: Quit",
			keyStyle.Render("j/k"), sepStyle.Render("•"),
			keyStyle.Render("Enter"), sepStyle.Render("•"),
			keyStyle.Render("Tab"), sepStyle.Render("•"),
			keyStyle.Render("/"), sepStyle.Render("•"),
			keyStyle.Render("Esc"), sepStyle.Render("•"),
			keyStyle.Render("q"))
	}

	return m.styles.HelpStyle().Render(helpText)
}

// resizeComponents handles window resize events
func (m *MainModel) resizeComponents() {
	headerHeight := lipgloss.Height(m.header.Render(m.width))
	// Account for: header + help line + list panel header row
	availableHeight := m.height - headerHeight - 1 - 1

	// Calculate panel dimensions
	contentWidth := m.width
	leftPanelWidth := int(float64(contentWidth) * 0.4)
	rightPanelWidth := contentWidth - leftPanelWidth

	// Resize list view
	m.listView.SetSize(leftPanelWidth-2, availableHeight)

	// Resize viewport for detail panel (accounting for borders and job header)
	m.detailViewport.Width = rightPanelWidth - 2
	m.detailViewport.Height = availableHeight - 1 // -1 for the job header row

	// Initialize detail content if not already set and we have items
	if m.detailViewport.TotalLineCount() == 0 {
		if selectedItem, ok := m.listView.GetSelectedItem(); ok {
			m.updateDetailContent(selectedItem)
		}
	}
}
