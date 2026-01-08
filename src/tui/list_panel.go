package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// renderListPanel renders the left panel with triage list
func (m MainModel) renderListPanel(width, height int) string {
	// Note: list size is set in resizeComponents(), not here during render

	// Render list with border
	listPanel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.styles.BorderColor).
		Width(width - 2).
		Height(height).
		Render(m.listView.Render())

	// Add column headers
	// Truncate to width-4 to account for padding (2 chars)
	// Only show "Novel" column if history exists
	var headerText string
	if len(m.noveltyMap) > 0 {
		headerText = "Seen | Novel | Message"
	} else {
		headerText = "Seen | Message"
	}
	truncatedHeaderText := Truncate(headerText, width-4, true)
	headerRow := lipgloss.NewStyle().
		Foreground(m.styles.PrimaryBlue).
		Bold(true).
		Width(width-2).
		Padding(0, 1).
		Render(truncatedHeaderText)

	return lipgloss.JoinVertical(lipgloss.Left, headerRow, listPanel)
}
