package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	// listRenderingOverhead accounts for padding added by bubbles/list and panel borders.
	// Breakdown: panel border (2) + list internal padding/margins (8) = 10 chars total.
	// This was determined empirically by measuring actual rendered output.
	listRenderingOverhead = 10
)

// Delegate renders triage items as table rows.
type Delegate struct {
	SeenWidth int
	styles    *StyleConfig
}

// NewDelegate creates a new triage table delegate with default styles
func NewDelegate() Delegate {
	return Delegate{
		SeenWidth: 4, // default minimum for "Seen" column
		styles:    DefaultStyles(),
	}
}

// SetColumnWidths sets the widths for the seen (recurrence) column
func (d *Delegate) SetColumnWidths(maxRecurrence int) {
	// Calculate width needed for recurrence (number of digits)
	d.SeenWidth = len(fmt.Sprintf("%d", maxRecurrence))
	if d.SeenWidth < 4 {
		d.SeenWidth = 4 // minimum width to align with "Seen" header
	}
}

// Height returns the height of a list item
func (d Delegate) Height() int {
	return 1
}

// Spacing returns spacing between items
func (d Delegate) Spacing() int {
	return 0
}

// Update handles item updates
func (d Delegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return nil
}

// getSnippetText returns the best text to show in the list snippet.
// It prefers RawMessage (original), falls back to Message (normalized),
// then to PreContext or PostContext if both are empty.
func getSnippetText(entry Item) string {
	// Try RawMessage first (original with actual values)
	if entry.Card.RawMessage != "" {
		cleanMsg := CleanLogText(entry.Card.RawMessage)
		if strings.TrimSpace(cleanMsg) != "" {
			return cleanMsg
		}
	}

	// Fall back to Message (normalized) for backwards compatibility
	cleanMsg := CleanLogText(entry.Card.NormalizedMsg)
	if strings.TrimSpace(cleanMsg) != "" {
		return cleanMsg
	}

	// Fall back to first non-empty line of PreContext
	for _, line := range entry.GetPreContext() {
		cleanLine := CleanLogText(line)
		if strings.TrimSpace(cleanLine) != "" {
			return cleanLine
		}
	}

	// Fall back to first non-empty line of PostContext
	for _, line := range entry.GetPostContext() {
		cleanLine := CleanLogText(line)
		if strings.TrimSpace(cleanLine) != "" {
			return cleanLine
		}
	}

	return ""
}

// Render renders a list item
func (d Delegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	entry, ok := item.(Item)
	if !ok {
		return
	}

	isSelected := index == m.Index()

	// Format "Seen" column (recurrence count)
	seenFmt := fmt.Sprintf("%%%dd", d.SeenWidth)
	seenCol := fmt.Sprintf(seenFmt, entry.GetRecurrence())

	// Format "Novel" column - star for novel, space otherwise
	var novelCol string
	if entry.IsNovel() {
		novelCol = "★"
	} else {
		novelCol = " "
	}

	// Calculate available width for snippet
	// Fixed columns: seen + novel (1) + separators (6: " │ " twice)
	fixedWidth := d.SeenWidth + 1 + 10
	availableWidth := m.Width() - fixedWidth - listRenderingOverhead

	var snippet string
	if availableWidth > 0 {
		// Get snippet text - use RawMessage, or fall back to Message/PreContext/PostContext
		snippetText := getSnippetText(entry)
		snippet = TruncateAndPad(snippetText, availableWidth, true)
	}

	// Tier 3 (noise) and low confidence cards (< 0.80) are dimmed
	isLowConfidence := entry.Card.ConfidenceScore < 0.80
	isNoise := entry.Tier == 3

	var rowStyle lipgloss.Style
	if isSelected {
		rowStyle = lipgloss.NewStyle().Bold(true).Foreground(d.styles.PrimaryBlue).Background(d.styles.SelectedColor)
	} else if isNoise || isLowConfidence {
		rowStyle = lipgloss.NewStyle().Foreground(d.styles.TextSecondary).Faint(true)
	} else {
		rowStyle = lipgloss.NewStyle().Foreground(d.styles.TextSecondary)
	}

	// Style the novel indicator
	novelStyle := lipgloss.NewStyle().Foreground(d.styles.NovelColor).Bold(true)

	// Build row: seen │ novel │ message
	if isSelected {
		// When selected, apply uniform style to entire row
		line := fmt.Sprintf("%s │ %s │ %s", seenCol, novelCol, snippet)
		fmt.Fprint(w, rowStyle.Render(line))
	} else {
		// When not selected, style novel indicator separately
		seenPart := rowStyle.Render(seenCol + " │ ")
		novelPart := novelStyle.Render(novelCol)
		msgPart := rowStyle.Render(" │ " + snippet)
		fmt.Fprint(w, seenPart+novelPart+msgPart)
	}
}
