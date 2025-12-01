package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	hashCol := truncateNoEllipsis(entry.Card.MessageHash, 5)                            // First 5 chars, no dots

	// Calculate available width for snippet
	// Separators: " │ " (3 chars) * 4 = 12 chars
	// Columns: rankWidth + 3 (conf) + recurWidth + 5 (hash)
	fixedWidth := d.RankWidth + 3 + d.RecurWidth + 5 + 12
	availableWidth := m.Width() - fixedWidth

	var snippet string
	if availableWidth > 0 {
		snippet = truncateString(entry.Card.Message, availableWidth)
	}

	line := fmt.Sprintf("%s │ %s │ %s │ %s │ %s",
		rankCol, confCol, recurCol, hashCol, snippet)

	style := lipgloss.NewStyle().Foreground(d.styles.TextSecondary)
	if isSelected {
		style = style.Bold(true).Foreground(d.styles.PrimaryBlue).Background(d.styles.SelectedColor)
	}

	fmt.Fprint(w, style.Render(line))
}

// truncateString truncates a string to maxLen with ellipsis
func truncateString(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if maxLen <= 0 {
		return ""
	}
	if len(s) > maxLen {
		if maxLen > 3 {
			return s[:maxLen-3] + "..."
		}
		return s[:maxLen]
	}
	// Pad to maxLen
	return s + strings.Repeat(" ", maxLen-len(s))
}

// truncateNoEllipsis truncates a string to maxLen without ellipsis, just cuts it
func truncateNoEllipsis(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) > maxLen {
		return s[:maxLen]
	}
	// Pad to maxLen
	return s + strings.Repeat(" ", maxLen-len(s))
}
