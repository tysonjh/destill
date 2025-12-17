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
	RankWidth  int
	RecurWidth int
	styles     *StyleConfig
}

// NewDelegate creates a new triage table delegate with default styles
func NewDelegate() Delegate {
	return Delegate{
		RankWidth:  2, // default minimum
		RecurWidth: 2, // default minimum
		styles:     DefaultStyles(),
	}
}

// SetColumnWidths sets the widths for rank and recurrence columns
func (d *Delegate) SetColumnWidths(maxRank, maxRecurrence int) {
	// Calculate width needed for rank (number of digits)
	d.RankWidth = len(fmt.Sprintf("%d", maxRank))
	if d.RankWidth < 2 {
		d.RankWidth = 2
	}

	// Calculate width needed for recurrence (number of digits)
	d.RecurWidth = len(fmt.Sprintf("%d", maxRecurrence))
	if d.RecurWidth < 2 {
		d.RecurWidth = 2
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

	rankFmt := fmt.Sprintf("%%%dd", d.RankWidth)   // e.g., "%3d" for 3-digit width
	recurFmt := fmt.Sprintf("%%%dd", d.RecurWidth) // e.g., "%4d" for 4-digit width

	rankNum := fmt.Sprintf(rankFmt, entry.Rank)              // dynamic width, right aligned
	recurCol := fmt.Sprintf(recurFmt, entry.GetRecurrence()) // dynamic width, right aligned

	// Format confidence score: show ".95" for < 1.0, "1.0" for 1.0
	var confCol string
	if entry.Card.ConfidenceScore >= 1.0 {
		confCol = "1.0"
	} else {
		confCol = fmt.Sprintf("%.2f", entry.Card.ConfidenceScore)[1:] // ".95" format
	}

	// Calculate available width for snippet
	// Fixed columns: rank + conf (3) + recurrence + separators (9)
	fixedWidth := d.RankWidth + 3 + d.RecurWidth + 9
	availableWidth := m.Width() - fixedWidth - listRenderingOverhead

	var snippet string
	if availableWidth > 0 {
		// Get snippet text - use RawMessage, or fall back to Message/PreContext/PostContext
		snippetText := getSnippetText(entry)
		snippet = TruncateAndPad(snippetText, availableWidth, true)
	}

	// Style rank number by tier (1=unique failures, 3=noise)
	var rankStyle lipgloss.Style
	switch entry.Tier {
	case 1: // Unique failures
		rankStyle = lipgloss.NewStyle().Foreground(d.styles.Tier1Color).Bold(true)
	case 3: // Noise
		rankStyle = lipgloss.NewStyle().Foreground(d.styles.Tier3Color).Faint(true)
	default:
		rankStyle = lipgloss.NewStyle().Foreground(d.styles.TextSecondary)
	}

	rankCol := rankStyle.Render(rankNum)

	// Build row: rank (colored) │ conf │ recur │ snippet
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

	// Build row with styled rank and rest of content
	restOfLine := fmt.Sprintf(" │ %s │ %s │ %s", confCol, recurCol, snippet)

	if isSelected {
		// When selected, apply uniform style to entire row
		line := fmt.Sprintf("%s │ %s │ %s │ %s", rankNum, confCol, recurCol, snippet)
		fmt.Fprint(w, rowStyle.Render(line))
	} else {
		// When not selected, keep rank colored by tier
		fmt.Fprint(w, rankCol+rowStyle.Render(restOfLine))
	}
}
