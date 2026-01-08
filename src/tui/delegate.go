package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"destill-agent/src/store"
)

const (
	// listRenderingOverhead accounts for padding added by bubbles/list and panel borders.
	// Breakdown: panel border (2) + list internal padding/margins (8) = 10 chars total.
	// This was determined empirically by measuring actual rendered output.
	listRenderingOverhead = 10
)

// Delegate renders triage items as table rows.
// NoveltyMap is looked up at render time to determine if a finding is novel.
type Delegate struct {
	SeenWidth   int
	styles      *StyleConfig
	NoveltyMap  *map[string]store.FindingNoveltyInfo // Pointer to model's novelty map
	HasHistory  bool                                  // True if novelty history exists (hide column if false)
}

// NewDelegate creates a new triage table delegate with default styles
func NewDelegate() Delegate {
	return Delegate{
		SeenWidth: 4, // default minimum for "Seen" column
		styles:    DefaultStyles(),
	}
}

// SetNoveltyMap sets the novelty map reference for render-time lookups.
// HasHistory is set to true only if the map contains entries (history exists).
func (d *Delegate) SetNoveltyMap(m *map[string]store.FindingNoveltyInfo) {
	d.NoveltyMap = m
	d.HasHistory = m != nil && len(*m) > 0
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

	// Format "Novel" column - only shown if history exists
	var novelCol string
	if d.HasHistory {
		isNovel := true
		if d.NoveltyMap != nil {
			if _, found := (*d.NoveltyMap)[entry.Card.MessageHash]; found {
				isNovel = false // Found in history = not novel
			}
		}
		if isNovel {
			novelCol = "★"
		} else {
			novelCol = " "
		}
	}

	// Calculate available width for snippet
	// Fixed columns: seen + separators, plus novel column if showing
	fixedWidth := d.SeenWidth + 5 // " │ " separator
	if d.HasHistory {
		fixedWidth += 1 + 5 // novel (1 char) + " │ " separator
	}
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

	// Build row: with or without novel column based on history availability
	if d.HasHistory {
		// Full row: seen │ novel │ message
		if isSelected {
			line := fmt.Sprintf("%s │ %s │ %s", seenCol, novelCol, snippet)
			fmt.Fprint(w, rowStyle.Render(line))
		} else {
			seenPart := rowStyle.Render(seenCol + " │ ")
			novelPart := novelStyle.Render(novelCol)
			msgPart := rowStyle.Render(" │ " + snippet)
			fmt.Fprint(w, seenPart+novelPart+msgPart)
		}
	} else {
		// No history: seen │ message (skip novel column)
		line := fmt.Sprintf("%s │ %s", seenCol, snippet)
		fmt.Fprint(w, rowStyle.Render(line))
	}
}
