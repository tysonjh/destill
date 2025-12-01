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
	// Account for: header + help line + panel borders (2 lines per panel)
	availableHeight := m.height - headerHeight - 2 - 2

	// Two-panel layout: Triage List (40%) | Context Detail (60%)
	leftPanelWidth := int(float64(m.width) * 0.4)
	rightPanelWidth := m.width - leftPanelWidth

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
	// Calculate panel dimensions
	leftPanelWidth := int(float64(m.width) * 0.4)
	rightPanelWidth := m.width - leftPanelWidth
	availableHeight := m.height - 7 // header(3) + help(1) + borders(2) + padding(1)

	// Resize list view
	m.listView.SetSize(leftPanelWidth-4, availableHeight)

	// Resize viewport for detail panel (accounting for borders and job header)
	m.detailViewport.Width = rightPanelWidth - 4
	m.detailViewport.Height = availableHeight - 4

	// Initialize detail content if not already set and we have items
	if m.detailViewport.TotalLineCount() == 0 {
		if selectedItem, ok := m.listView.GetSelectedItem(); ok {
			m.updateDetailContent(selectedItem)
		}
	}
}
