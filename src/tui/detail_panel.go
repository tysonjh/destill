package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderDetail renders the detail content for a triage item
func (m MainModel) renderDetail(item Item, maxWidth int) string {
	content := strings.Builder{}

	// Detail Header - truncate hash to keep header on one line
	// Format: "Hash: XXXXX | Severity: XXX | Job: XXX"
	// We truncate the hash to ensure the whole header fits within maxWidth
	shortHash := item.Card.MessageHash
	if len(shortHash) > 12 {
		shortHash = shortHash[:12] // Show first 12 chars of hash
	}
	headerText := fmt.Sprintf("Hash: %s | Severity: %s | Job: %s",
		shortHash,
		item.Card.Severity,
		Truncate(item.Card.JobName, maxWidth-40, true)) // Reserve space for hash/severity
	// Truncate the entire header if still too long
	headerText = Truncate(headerText, maxWidth, true)
	header := lipgloss.NewStyle().
		Foreground(m.styles.PrimaryBlue).
		Bold(true).
		Render(headerText)
	fmt.Fprintf(&content, "%s\n\n", header)

	// Pre-context - clean and wrap each line
	preContext := item.GetPreContext()
	if len(preContext) > 0 {
		fmt.Fprintln(&content, lipgloss.NewStyle().Foreground(m.styles.TextSecondary).Bold(true).Render("Pre-Context:"))
		for _, line := range preContext {
			// Clean Buildkite escape sequences and normalize
			cleanLine := CleanLogText(line)
			if strings.TrimSpace(cleanLine) != "" {
				// Wrap line before styling
				wrapped := Wrap(cleanLine, maxWidth)
				fmt.Fprint(&content, lipgloss.NewStyle().Foreground(m.styles.TextSecondary).Faint(true).Render(wrapped))
				fmt.Fprintln(&content)
			}
		}
		fmt.Fprintln(&content, "")
	}

	// Error Message (Highlight) - clean and wrap before styling
	fmt.Fprintln(&content, lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Bold(true).Render("ERROR:"))
	cleanMessage := CleanLogText(item.Card.Message)
	wrappedError := Wrap(cleanMessage, maxWidth)
	fmt.Fprint(&content, lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FF0000")).
		Background(lipgloss.Color("#2D0000")).
		Render(wrappedError))
	fmt.Fprintln(&content, "")
	fmt.Fprintln(&content, "")

	// Post-context - clean and wrap each line
	postContext := item.GetPostContext()
	if len(postContext) > 0 {
		fmt.Fprintln(&content, lipgloss.NewStyle().Foreground(m.styles.TextSecondary).Bold(true).Render("Post-Context:"))
		for _, line := range postContext {
			// Clean Buildkite escape sequences and normalize
			cleanLine := CleanLogText(line)
			if strings.TrimSpace(cleanLine) != "" {
				// Wrap line before styling
				wrapped := Wrap(cleanLine, maxWidth)
				fmt.Fprint(&content, lipgloss.NewStyle().Foreground(m.styles.TextSecondary).Faint(true).Render(wrapped))
				fmt.Fprintln(&content)
			}
		}
	}

	return content.String()
}

// updateDetailContent updates the viewport with content from the selected item
func (m *MainModel) updateDetailContent(item Item) {
	// The viewport's width is the max width for the content.
	// Subtract a small amount for internal padding.
	maxWidth := m.detailViewport.Width - 2 // 1 char padding on each side
	content := m.renderDetail(item, maxWidth)
	m.detailViewport.SetContent(content)
}

// renderDetailPanel renders the right panel with detail viewport
func (m MainModel) renderDetailPanel(width, height int) string {
	if selectedItem, ok := m.listView.GetSelectedItem(); ok {
		// Add header row with job name
		// Truncate to width-4 to account for padding (2 chars)
		jobText := fmt.Sprintf("Job: %s", selectedItem.Card.JobName)
		truncatedJobText := Truncate(jobText, width-4, true)
		headerRow := lipgloss.NewStyle().
			Foreground(m.styles.PrimaryBlue).
			Bold(true).
			Width(width-2).
			Padding(0, 1).
			Render(truncatedJobText)

		// Show viewport content
		borderStyle := m.styles.BorderColor
		if m.detailFocused {
			borderStyle = m.styles.AccentBlue
		}

		detailWithHeader := lipgloss.JoinVertical(lipgloss.Left, headerRow,
			lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(borderStyle).
				Width(width-2).
				Height(height).
				Render(m.detailViewport.View()))

		return detailWithHeader
	}

	// No selection - show empty state
	placeholderRow := lipgloss.NewStyle().
		Foreground(m.styles.TextSecondary).
		Width(width-2).
		Padding(0, 1).
		Render(" ")

	emptyStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.styles.BorderColor).
		Width(width).
		Height(height).
		Align(lipgloss.Center, lipgloss.Center).
		Foreground(m.styles.TextSecondary).
		Faint(true)

	return lipgloss.JoinVertical(lipgloss.Left, placeholderRow, emptyStyle.Render("← Navigate list to view details"))
}
