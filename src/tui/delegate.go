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

// NewDelegateWithStyles creates a new delegate with custom styles
func NewDelegateWithStyles(styles *StyleConfig) Delegate {
	return Delegate{
		RankWidth:  2,
		RecurWidth: 2,
		styles:     styles,
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
// It prefers Message, but falls back to PreContext or PostContext if Message is empty.
func getSnippetText(entry Item) string {
	// Try Message first - clean Buildkite escape sequences and check if meaningful
	cleanMsg := CleanLogText(entry.Card.Message)
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

	// Build table row
	rankFmt := fmt.Sprintf("%%%dd", d.RankWidth)   // e.g., "%3d" for 3-digit width
	recurFmt := fmt.Sprintf("%%%dd", d.RecurWidth) // e.g., "%4d" for 4-digit width

	rankCol := fmt.Sprintf(rankFmt, entry.Rank)                                         // dynamic width, right aligned
	confCol := fmt.Sprintf("%-3s", fmt.Sprintf("%.2f", entry.Card.ConfidenceScore)[1:]) // 4 chars, left aligned, decimal point on left (.95)
	recurCol := fmt.Sprintf(recurFmt, entry.GetRecurrence())                            // dynamic width, right aligned
	hashCol := TruncateAndPad(entry.Card.MessageHash, 5, false)                         // First 5 chars, no ellipsis

	// Calculate available width for snippet
	// Fixed columns: rank + conf (3) + recurrence + hash (5) + separators (12)
	fixedWidth := d.RankWidth + 3 + d.RecurWidth + 5 + 12
	availableWidth := m.Width() - fixedWidth - listRenderingOverhead

	var snippet string
	if availableWidth > 0 {
		// Get snippet text - use Message, or fall back to PreContext/PostContext
		snippetText := getSnippetText(entry)
		snippet = TruncateAndPad(snippetText, availableWidth, true)
	}

	line := fmt.Sprintf("%s │ %s │ %s │ %s │ %s",
		rankCol, confCol, recurCol, hashCol, snippet)

	style := lipgloss.NewStyle().Foreground(d.styles.TextSecondary)
	if isSelected {
		style = style.Bold(true).Foreground(d.styles.PrimaryBlue).Background(d.styles.SelectedColor)
	}

	fmt.Fprint(w, style.Render(line))
}
