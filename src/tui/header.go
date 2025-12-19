package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// JobInfo contains information about a job including its failure status.
type JobInfo struct {
	Name   string
	Failed bool
}

// Header represents the top status bar component.
type Header struct {
	projectStatus      string
	selectedFilter     string // Formatted display string
	rawFilterName      string // Raw filter name for comparison
	availableJobs      []JobInfo
	currentFilterIndex int
	searchQuery        string
	searchMode         bool
	styles             *StyleConfig

	// Streaming status
	loadStatus         LoadStatus
	cardCount          int
	lowConfidenceCount int
	jobCount           int
	pendingCount       int

	// Tier counts
	uniqueCount int
	noiseCount  int
	tierFilter  int // 0=all (default), 1=unique only, 2=noise only

	// View mode - controls which sections are shown
	viewMode ViewMode
}

// NewHeaderWithStyles creates a new header with custom styles
func NewHeaderWithStyles(projectStatus string, availableJobs []JobInfo, styles *StyleConfig) Header {
	h := Header{
		projectStatus:      projectStatus,
		availableJobs:      availableJobs,
		currentFilterIndex: 0,
		rawFilterName:      "ALL",
		styles:             styles,
	}
	h.selectedFilter = h.formatFilterDisplay("ALL", false)
	return h
}

// GetFilter returns the current filter name (unformatted)
func (h Header) GetFilter() string {
	return h.rawFilterName
}

// formatFilterDisplay formats the filter display string with index and optional red color for failed jobs
func (h Header) formatFilterDisplay(filterName string, failed bool) string {
	displayName := filterName
	if failed {
		// Apply red color to failed job name
		redStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
		displayName = redStyle.Render(filterName)
	}
	return fmt.Sprintf("%s [%d/%d]", displayName, h.currentFilterIndex, len(h.availableJobs))
}

// CycleFilter cycles to the next filter
func (h *Header) CycleFilter() {
	totalFilters := 1 + len(h.availableJobs) // ALL + jobs
	h.currentFilterIndex = (h.currentFilterIndex + 1) % totalFilters
	if h.currentFilterIndex == 0 {
		h.rawFilterName = "ALL"
		h.selectedFilter = h.formatFilterDisplay("ALL", false)
	} else {
		jobInfo := h.availableJobs[h.currentFilterIndex-1]
		h.rawFilterName = jobInfo.Name
		h.selectedFilter = h.formatFilterDisplay(jobInfo.Name, jobInfo.Failed)
	}
}

// CycleFilterBackward cycles to the previous filter
func (h *Header) CycleFilterBackward() {
	totalFilters := 1 + len(h.availableJobs) // ALL + jobs
	h.currentFilterIndex = (h.currentFilterIndex - 1 + totalFilters) % totalFilters
	if h.currentFilterIndex == 0 {
		h.rawFilterName = "ALL"
		h.selectedFilter = h.formatFilterDisplay("ALL", false)
	} else {
		jobInfo := h.availableJobs[h.currentFilterIndex-1]
		h.rawFilterName = jobInfo.Name
		h.selectedFilter = h.formatFilterDisplay(jobInfo.Name, jobInfo.Failed)
	}
}

// ResetFilter resets the filter to ALL
func (h *Header) ResetFilter() {
	h.currentFilterIndex = 0
	h.rawFilterName = "ALL"
	h.selectedFilter = h.formatFilterDisplay("ALL", false)
}

// SetInitialFilter sets the filter to a specific job by name.
// This is used during initialization to default to the first failed job.
func (h *Header) SetInitialFilter(jobName string, failed bool) {
	// Find the job index
	for i, job := range h.availableJobs {
		if job.Name == jobName {
			h.currentFilterIndex = i + 1 // +1 because index 0 is "ALL"
			h.rawFilterName = jobName
			h.selectedFilter = h.formatFilterDisplay(jobName, failed)
			return
		}
	}
	// Job not found, stay on ALL
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

// SetLowConfidenceCount updates the low confidence cards count
func (h *Header) SetLowConfidenceCount(count int) {
	h.lowConfidenceCount = count
}

// SetTierCounts updates the tier counts for display
func (h *Header) SetTierCounts(unique, noise int) {
	h.uniqueCount = unique
	h.noiseCount = noise
}

// SetTierFilter updates the current tier filter
func (h *Header) SetTierFilter(filter int) {
	h.tierFilter = filter
}

// SetViewMode updates the current view mode
func (h *Header) SetViewMode(mode ViewMode) {
	h.viewMode = mode
}

// AddJob adds a new job to the available jobs list
func (h *Header) AddJob(jobName string, failed bool) {
	// Check if already exists - if so, update failed status
	for i, j := range h.availableJobs {
		if j.Name == jobName {
			// Update failed status if this job failed
			if failed && !h.availableJobs[i].Failed {
				h.availableJobs[i].Failed = true
			}
			return
		}
	}

	// Insert new job - failed jobs go first
	newJob := JobInfo{Name: jobName, Failed: failed}
	if failed {
		// Insert failed job at the beginning (before non-failed jobs)
		insertIndex := 0
		for i, j := range h.availableJobs {
			if !j.Failed {
				insertIndex = i
				break
			}
			insertIndex = i + 1
		}
		h.availableJobs = append(h.availableJobs[:insertIndex], append([]JobInfo{newJob}, h.availableJobs[insertIndex:]...)...)
	} else {
		// Non-failed jobs go at the end
		h.availableJobs = append(h.availableJobs, newJob)
	}

	// Update filter display to reflect new job count
	if h.currentFilterIndex == 0 {
		h.selectedFilter = h.formatFilterDisplay("ALL", false)
	} else if h.currentFilterIndex-1 < len(h.availableJobs) {
		jobInfo := h.availableJobs[h.currentFilterIndex-1]
		h.selectedFilter = h.formatFilterDisplay(jobInfo.Name, jobInfo.Failed)
	}
}

// Render renders the header
func (h Header) Render(width int) string {
	// Project status section with loading indicator
	statusStyle := lipgloss.NewStyle().
		Foreground(h.styles.PrimaryBlue).
		Bold(true).
		Padding(0, 2)

	status := statusStyle.Render(h.projectStatus)

	// Pending indicator (shows when new cards waiting)
	var pending string
	if h.pendingCount > 0 {
		pendingStyle := lipgloss.NewStyle().
			Foreground(h.styles.AccentYellow).
			Bold(true).
			Padding(0, 1)
		pending = pendingStyle.Render(fmt.Sprintf("⚡ %d new (r)", h.pendingCount))
	}

	// Build left section based on view mode
	var leftSection string

	if h.viewMode == ViewSummary || h.viewMode == ViewTests {
		// Summary/Tests view: just show status and pending (no filters)
		leftSection = lipgloss.JoinHorizontal(lipgloss.Left, status, pending)
	} else {
		// Logs view: show all controls

		// Tier counts section - colored numbers matching delegate tier colors
		// Format: Unique:3 Noise:8 with active filter highlighted
		// Note: Using TextSecondary for inactive instead of Faint() for better terminal compatibility
		uniqueStyle := lipgloss.NewStyle().Foreground(h.styles.Tier1Color)
		noiseStyle := lipgloss.NewStyle().Foreground(h.styles.Tier3Color)

		// Highlight active tier filter with bold, dim inactive
		switch h.tierFilter {
		case 0: // All (default)
			uniqueStyle = uniqueStyle.Bold(true)
			noiseStyle = noiseStyle.Bold(true)
		case 1: // Unique only
			uniqueStyle = uniqueStyle.Bold(true)
			noiseStyle = lipgloss.NewStyle().Foreground(h.styles.TextSecondary)
		case 2: // Noise only
			uniqueStyle = lipgloss.NewStyle().Foreground(h.styles.TextSecondary)
			noiseStyle = noiseStyle.Bold(true)
		}

		unique := uniqueStyle.Render(fmt.Sprintf("Unique:%d", h.uniqueCount))
		noise := noiseStyle.Render(fmt.Sprintf("Noise:%d", h.noiseCount))

		tierStyle := lipgloss.NewStyle().Padding(0, 1)
		tiers := tierStyle.Render(fmt.Sprintf("│ %s %s │", unique, noise))

		// Filter section - truncate if necessary to prevent wrapping
		filterStyle := lipgloss.NewStyle().
			Foreground(h.styles.PrimaryBlue).
			Bold(true).
			Padding(0, 2).
			MaxWidth(width / 4) // Limit filter width to prevent wrapping

		filter := filterStyle.Render(fmt.Sprintf("Job: %s", h.selectedFilter))

		// Search section
		var searchText string
		if h.searchMode {
			searchText = fmt.Sprintf("Search: %s_", h.searchQuery)
		} else if h.searchQuery != "" {
			searchText = fmt.Sprintf("Search: %s", h.searchQuery)
		} else {
			searchText = "[/] search"
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
		leftSection = lipgloss.JoinHorizontal(lipgloss.Left, status, pending, tiers, filter, search)
	}

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
