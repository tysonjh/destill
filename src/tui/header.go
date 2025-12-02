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

	// Streaming status
	loadStatus   LoadStatus
	cardCount    int
	jobCount     int
	pendingCount int
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

// SetLoadStatus updates the loading status display
func (h *Header) SetLoadStatus(status LoadStatus, cardCount, jobCount int) {
	h.loadStatus = status
	h.cardCount = cardCount
	h.jobCount = jobCount
}

// SetPendingCount updates the pending cards count
func (h *Header) SetPendingCount(count int) {
	h.pendingCount = count
}

// AddJob adds a new job to the available jobs list
func (h *Header) AddJob(jobName string) {
	// Check if already exists
	for _, j := range h.availableJobs {
		if j == jobName {
			return
		}
	}
	h.availableJobs = append(h.availableJobs, jobName)
	// Update filter display to reflect new job count
	h.selectedFilter = h.formatFilterDisplay(h.rawFilterName)
}

// Render renders the header
func (h Header) Render(width int) string {
	// Project status section with loading indicator
	statusStyle := lipgloss.NewStyle().
		Foreground(h.styles.PrimaryBlue).
		Bold(true).
		Padding(0, 2)

	var statusIcon string
	switch h.loadStatus {
	case StatusLoading:
		statusIcon = "⏳"
	case StatusComplete:
		statusIcon = "✅"
	case StatusError:
		statusIcon = "❌"
	default:
		statusIcon = "📊"
	}

	statusText := fmt.Sprintf("%s %s", statusIcon, h.projectStatus)
	if h.cardCount > 0 {
		statusText = fmt.Sprintf("%s (%d cards, %d jobs)", statusText, h.cardCount, h.jobCount)
	}
	status := statusStyle.Render(statusText)

	// Pending indicator (shows when new cards waiting)
	var pending string
	if h.pendingCount > 0 {
		pendingStyle := lipgloss.NewStyle().
			Foreground(h.styles.AccentYellow).
			Bold(true).
			Padding(0, 1)
		pending = pendingStyle.Render(fmt.Sprintf("⚡ %d new (r)", h.pendingCount))
	}

	// Filter section - truncate if necessary to prevent wrapping
	filterStyle := lipgloss.NewStyle().
		Foreground(h.styles.PrimaryBlue).
		Bold(true).
		Padding(0, 2).
		MaxWidth(width / 4) // Limit filter width to prevent wrapping

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
		MaxWidth(width / 4) // Limit search width to prevent wrapping
	if h.searchMode {
		searchStyle = searchStyle.Foreground(h.styles.PrimaryBlue)
	}

	search := searchStyle.Render(searchText)

	// Combine sections
	leftSection := lipgloss.JoinHorizontal(lipgloss.Left, status, pending, filter, search)

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
