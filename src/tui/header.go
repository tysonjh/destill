package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// Header represents the top status bar component.
type Header struct {
	projectStatus      string
	selectedFilter     string // Formatted display string
	rawFilterName      string // Raw filter name for comparison
	availableJobs      []string
	currentFilterIndex int
	searchQuery        string
	searchMode         bool
	styles             *StyleConfig
}

// NewHeader creates a new header with default styles
func NewHeader(projectStatus string, availableJobs []string) Header {
	h := Header{
		projectStatus:      projectStatus,
		availableJobs:      availableJobs,
		currentFilterIndex: 0,
		rawFilterName:      "ALL",
		styles:             DefaultStyles(),
	}
	h.selectedFilter = h.formatFilterDisplay("ALL")
	return h
}

// NewHeaderWithStyles creates a new header with custom styles
func NewHeaderWithStyles(projectStatus string, availableJobs []string, styles *StyleConfig) Header {
	h := Header{
		projectStatus:      projectStatus,
		availableJobs:      availableJobs,
		currentFilterIndex: 0,
		rawFilterName:      "ALL",
		styles:             styles,
	}
	h.selectedFilter = h.formatFilterDisplay("ALL")
	return h
}

// SetFilter sets the current filter
func (h *Header) SetFilter(filter string) {
	h.selectedFilter = filter
}

// GetFilter returns the current filter name (unformatted)
func (h Header) GetFilter() string {
	return h.rawFilterName
}

// formatFilterDisplay formats the filter display string with index
func (h Header) formatFilterDisplay(filterName string) string {
	return fmt.Sprintf("%s [%d/%d]", filterName, h.currentFilterIndex, len(h.availableJobs))
}

// CycleFilter cycles to the next filter
func (h *Header) CycleFilter() {
	filters := append([]string{"ALL"}, h.availableJobs...)
	h.currentFilterIndex = (h.currentFilterIndex + 1) % len(filters)
	h.rawFilterName = filters[h.currentFilterIndex]
	h.selectedFilter = h.formatFilterDisplay(h.rawFilterName)
}

// CycleFilterBackward cycles to the previous filter
func (h *Header) CycleFilterBackward() {
	filters := append([]string{"ALL"}, h.availableJobs...)
	h.currentFilterIndex = (h.currentFilterIndex - 1 + len(filters)) % len(filters)
	h.rawFilterName = filters[h.currentFilterIndex]
	h.selectedFilter = h.formatFilterDisplay(h.rawFilterName)
}

// ResetFilter resets the filter to ALL
func (h *Header) ResetFilter() {
	h.currentFilterIndex = 0
	h.rawFilterName = "ALL"
	h.selectedFilter = h.formatFilterDisplay("ALL")
}

// SetSearch updates the search state
func (h *Header) SetSearch(query string, mode bool) {
	h.searchQuery = query
	h.searchMode = mode
}

// Render renders the header
func (h Header) Render(width int) string {
	// Project status section
	statusStyle := lipgloss.NewStyle().
		Foreground(h.styles.PrimaryBlue).
		Bold(true).
		Padding(0, 2)

	status := statusStyle.Render(fmt.Sprintf("📊 %s", h.projectStatus))

	// Filter section - truncate if necessary to prevent wrapping
	filterStyle := lipgloss.NewStyle().
		Foreground(h.styles.PrimaryBlue).
		Bold(true).
		Padding(0, 2).
		MaxWidth(width / 3) // Limit filter width to prevent wrapping

	filter := filterStyle.Render(fmt.Sprintf("⚙️ Job: %s", h.selectedFilter))

	// Search section
	var searchText string
	if h.searchMode {
		searchText = fmt.Sprintf("🔍 Search: %s█", h.searchQuery)
	} else if h.searchQuery != "" {
		searchText = fmt.Sprintf("🔍 Search: %s", h.searchQuery)
	} else {
		searchText = "🔍 [/] to search"
	}

	searchStyle := lipgloss.NewStyle().
		Foreground(h.styles.TextSecondary).
		Padding(0, 2).
		MaxWidth(width / 3) // Limit search width to prevent wrapping
	if h.searchMode {
		searchStyle = searchStyle.Foreground(h.styles.PrimaryBlue)
	}

	search := searchStyle.Render(searchText)

	// Combine sections
	leftSection := lipgloss.JoinHorizontal(lipgloss.Left, status, filter, search)

	// Create header bar - no background to ensure visibility on any terminal
	// Note: BorderBottom adds 2 chars (left and right corners), so content width is width - 2
	headerStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		BorderForeground(h.styles.BorderColor).
		Width(width - 2)

	// Space out left and right sections
	spacer := lipgloss.NewStyle().Width(width - 2 - lipgloss.Width(leftSection)).Render("")

	content := lipgloss.JoinHorizontal(lipgloss.Left, leftSection, spacer)

	return headerStyle.Render(content)
}
