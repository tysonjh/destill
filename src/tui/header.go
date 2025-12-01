package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// Header represents the top status bar component.
type Header struct {
	projectStatus  string
	selectedFilter string
	availableJobs  []string
	searchQuery    string
	searchMode     bool
	styles         *StyleConfig
}

// NewHeader creates a new header with default styles
func NewHeader(projectStatus string, availableJobs []string) Header {
	return Header{
		projectStatus:  projectStatus,
		selectedFilter: "ALL",
		availableJobs:  availableJobs,
		styles:         DefaultStyles(),
	}
}

// NewHeaderWithStyles creates a new header with custom styles
func NewHeaderWithStyles(projectStatus string, availableJobs []string, styles *StyleConfig) Header {
	return Header{
		projectStatus:  projectStatus,
		selectedFilter: "ALL",
		availableJobs:  availableJobs,
		styles:         styles,
	}
}

// SetFilter sets the current filter
func (h *Header) SetFilter(filter string) {
	h.selectedFilter = filter
}

// GetFilter returns the current filter
func (h Header) GetFilter() string {
	return h.selectedFilter
}

// CycleFilter cycles to the next filter
func (h *Header) CycleFilter() {
	filters := append([]string{"ALL"}, h.availableJobs...)
	currentIndex := 0
	for i, f := range filters {
		if f == h.selectedFilter {
			currentIndex = i
			break
		}
	}
	nextIndex := (currentIndex + 1) % len(filters)
	h.selectedFilter = filters[nextIndex]
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

	// Filter section
	filterStyle := lipgloss.NewStyle().
		Foreground(h.styles.PrimaryBlue).
		Bold(true).
		Padding(0, 2)

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
		Padding(0, 2)
	if h.searchMode {
		searchStyle = searchStyle.Foreground(h.styles.PrimaryBlue)
	}

	search := searchStyle.Render(searchText)

	// Combine sections
	leftSection := lipgloss.JoinHorizontal(lipgloss.Left, status, filter, search)

	// Create header bar with background
	headerStyle := lipgloss.NewStyle().
		Background(h.styles.DarkBackground).
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		BorderForeground(h.styles.BorderColor).
		Width(width)

	// Space out left and right sections
	spacer := lipgloss.NewStyle().Width(width - lipgloss.Width(leftSection)).Render("")

	content := lipgloss.JoinHorizontal(lipgloss.Left, leftSection, spacer)

	return headerStyle.Render(content)
}
