package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// panelDimensions holds calculated layout dimensions
type panelDimensions struct {
	availableHeight int
	leftPanelWidth  int
	rightPanelWidth int
}

// calculateDimensions computes panel sizes based on terminal dimensions.
// This centralizes the layout math to ensure consistency across render and resize.
func (m MainModel) calculateDimensions() panelDimensions {
	headerHeight := lipgloss.Height(m.header.Render(m.width))
	// Account for: header + help line (1) + panel column header row (1) + panel borders (2)
	availableHeight := m.height - headerHeight - 1 - 1 - 2

	// Two-panel layout: Triage List (40%) | Context Detail (60%)
	leftPanelWidth := int(float64(m.width) * 0.4)
	rightPanelWidth := m.width - leftPanelWidth

	return panelDimensions{
		availableHeight: availableHeight,
		leftPanelWidth:  leftPanelWidth,
		rightPanelWidth: rightPanelWidth,
	}
}

// View renders the complete TUI layout
func (m MainModel) View() string {
	if !m.ready {
		return "\n  Initializing..."
	}

	// Render header
	header := m.header.Render(m.width)

	// If we're still loading and have no items yet, show progress with logo
	if m.status == StatusLoading && len(m.items) == 0 {
		progressView := m.progress.View()
		// Center the progress view in the available space
		centeredProgress := lipgloss.NewStyle().
			Width(m.width).
			Align(lipgloss.Center).
			PaddingTop(2).
			Render(progressView)
		return lipgloss.JoinVertical(lipgloss.Left, header, centeredProgress)
	}

	// Render based on current view mode
	switch m.viewMode {
	case ViewSummary:
		return lipgloss.JoinVertical(lipgloss.Left, header, m.summaryModel.View())
	case ViewTests:
		return lipgloss.JoinVertical(lipgloss.Left, header, m.testsModel.View())
	default:
		// ViewLogs - existing behavior
		return m.renderLogsView(header)
	}
}

// renderLogsView renders the logs view (original TUI layout)
func (m MainModel) renderLogsView(header string) string {
	// Calculate panel dimensions
	dims := m.calculateDimensions()

	// Render panels
	leftPanel := m.renderListPanel(dims.leftPanelWidth, dims.availableHeight)
	rightPanel := m.renderDetailPanel(dims.rightPanelWidth, dims.availableHeight)

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

	// View-specific help
	switch m.viewMode {
	case ViewSummary:
		helpText = fmt.Sprintf("%s: Tests %s %s: Logs %s %s: Quit",
			keyStyle.Render("t"), sepStyle.Render("•"),
			keyStyle.Render("l"), sepStyle.Render("•"),
			keyStyle.Render("q"))
	case ViewTests:
		helpText = fmt.Sprintf("%s: Scroll %s %s: Summary %s %s: Logs %s %s: Quit",
			keyStyle.Render("j/k"), sepStyle.Render("•"),
			keyStyle.Render("s"), sepStyle.Render("•"),
			keyStyle.Render("l"), sepStyle.Render("•"),
			keyStyle.Render("q"))
	default: // ViewLogs
		if m.detailFocused {
			helpText = fmt.Sprintf("%s: Scroll %s %s: Back %s %s: Summary %s %s: Quit",
				keyStyle.Render("j/k"), sepStyle.Render("•"),
				keyStyle.Render("Esc"), sepStyle.Render("•"),
				keyStyle.Render("s"), sepStyle.Render("•"),
				keyStyle.Render("q"))
		} else {
			helpText = fmt.Sprintf("%s: Nav %s %s: All/Unique/Noise %s %s: Summary %s %s: Job %s %s %s",
				keyStyle.Render("j/k"), sepStyle.Render("•"),
				keyStyle.Render("0/1/2"), sepStyle.Render("•"),
				keyStyle.Render("s"), sepStyle.Render("•"),
				keyStyle.Render("Tab"), sepStyle.Render("•"),
				keyStyle.Render("/"), keyStyle.Render("q"))
		}
	}

	return m.styles.HelpStyle().Render(helpText)
}

// resizeComponents handles window resize events
func (m *MainModel) resizeComponents() {
	dims := m.calculateDimensions()

	// Resize list view (accounting for panel borders)
	m.listView.SetSize(dims.leftPanelWidth-2, dims.availableHeight)

	// Resize viewport for detail panel (accounting for borders and job header)
	m.detailViewport.Width = dims.rightPanelWidth - 2
	m.detailViewport.Height = dims.availableHeight - 1 // -1 for the job header row

	// Resize summary and tests views
	headerHeight := lipgloss.Height(m.header.Render(m.width))
	availableForViews := m.height - headerHeight - 2
	m.summaryModel.SetSize(m.width, availableForViews)
	m.testsModel.SetSize(m.width, availableForViews)

	// Initialize detail content if not already set and we have items
	if m.detailViewport.TotalLineCount() == 0 {
		if selectedItem, ok := m.listView.GetSelectedItem(); ok {
			m.updateDetailContent(selectedItem)
		}
	}
}
