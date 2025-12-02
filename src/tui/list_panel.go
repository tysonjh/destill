package tui

import (
	"fmt"

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
	delegate := m.listView.GetDelegate()
	rankHeader := fmt.Sprintf("%*s", delegate.RankWidth, "Rk")
	recurHeader := fmt.Sprintf("%*s", delegate.RecurWidth, "Rc")

	// Truncate to width-4 to account for padding (2 chars)
	headerText := fmt.Sprintf("%s │ Conf │ %s │ Message", rankHeader, recurHeader)
	truncatedHeaderText := Truncate(headerText, width-4, true)
	headerRow := lipgloss.NewStyle().
		Foreground(m.styles.PrimaryBlue).
		Bold(true).
		Width(width-2).
		Padding(0, 1).
		Render(truncatedHeaderText)

	return lipgloss.JoinVertical(lipgloss.Left, headerRow, listPanel)
}
