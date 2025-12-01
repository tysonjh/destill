package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// renderListPanel renders the left panel with triage list
func (m MainModel) renderListPanel(width, height int) string {
	// Set list size (accounting for panel borders)
	m.listView.SetSize(width-2, height)

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

	headerRow := lipgloss.NewStyle().
		Foreground(m.styles.PrimaryBlue).
		Bold(true).
		Width(width-2).
		Padding(0, 1).
		Render(fmt.Sprintf("%s │ Conf │ %s │ Hash  │ Message", rankHeader, recurHeader))

	return lipgloss.JoinVertical(lipgloss.Left, headerRow, listPanel)
}
