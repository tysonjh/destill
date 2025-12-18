package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"destill-agent/src/contracts"
)

// SummaryModel displays a build summary overview.
type SummaryModel struct {
	width       int
	height      int
	styles      *StyleConfig
	buildStatus string
	buildNumber string
	buildURL    string

	// Job counts
	failedJobCount  int
	passedJobCount  int
	otherJobCount   int

	// Log findings
	uniqueCount int
	noiseCount  int

	// Test results
	testSummary    *contracts.TestSummary
	hasTestResults bool
}

// NewSummaryModel creates a new summary view model.
func NewSummaryModel(styles *StyleConfig) SummaryModel {
	return SummaryModel{
		styles:      styles,
		buildStatus: "unknown",
	}
}

// SetBuildInfo updates the build information.
func (m *SummaryModel) SetBuildInfo(status, number, url string) {
	m.buildStatus = status
	m.buildNumber = number
	m.buildURL = url
}

// SetJobCounts updates the job counts.
func (m *SummaryModel) SetJobCounts(failed, passed, other int) {
	m.failedJobCount = failed
	m.passedJobCount = passed
	m.otherJobCount = other
}

// SetLogFindings updates the log finding counts.
func (m *SummaryModel) SetLogFindings(unique, noise int) {
	m.uniqueCount = unique
	m.noiseCount = noise
}

// SetTestSummary updates the test summary.
func (m *SummaryModel) SetTestSummary(summary *contracts.TestSummary) {
	m.testSummary = summary
	m.hasTestResults = summary != nil && summary.TotalTests > 0
}

// SetSize sets the view dimensions.
func (m *SummaryModel) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// Update handles messages.
func (m SummaryModel) Update(msg tea.Msg) (SummaryModel, tea.Cmd) {
	return m, nil
}

// View renders the summary.
func (m SummaryModel) View() string {
	if m.width == 0 {
		return ""
	}

	var sections []string

	// Title bar
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(m.styles.PrimaryBlue).
		Width(m.width - 4).
		Align(lipgloss.Center).
		Padding(1, 0)
	sections = append(sections, titleStyle.Render("Build Summary"))

	// Build status section
	statusSection := m.renderStatusSection()
	sections = append(sections, statusSection)

	// Two-column layout for Test Results and Log Findings
	columnsSection := m.renderColumnsSection()
	sections = append(sections, columnsSection)

	// Key bindings footer
	footer := m.renderFooter()
	sections = append(sections, footer)

	// Join all sections
	content := lipgloss.JoinVertical(lipgloss.Left, sections...)

	// Wrap in a box
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.styles.BorderColor).
		Padding(0, 2).
		Width(m.width - 2)

	return boxStyle.Render(content)
}

func (m SummaryModel) renderStatusSection() string {
	// Status line
	statusColor := m.styles.TextSecondary
	statusText := "UNKNOWN"
	if m.buildStatus == "failed" {
		statusColor = lipgloss.Color("#FF0000")
		statusText = "FAILED"
	} else if m.buildStatus == "passed" {
		statusColor = lipgloss.Color("#00FF00")
		statusText = "PASSED"
	}

	statusStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(statusColor)

	labelStyle := lipgloss.NewStyle().
		Foreground(m.styles.TextSecondary)

	// Build info line
	buildInfo := fmt.Sprintf("Status: %s", statusStyle.Render(statusText))
	if m.buildNumber != "" {
		buildInfo += fmt.Sprintf("    Build #%s", m.buildNumber)
	}

	// Job counts line
	var jobParts []string
	if m.failedJobCount > 0 {
		jobParts = append(jobParts, fmt.Sprintf("%d failed", m.failedJobCount))
	}
	if m.passedJobCount > 0 {
		jobParts = append(jobParts, fmt.Sprintf("%d passed", m.passedJobCount))
	}
	if m.otherJobCount > 0 {
		jobParts = append(jobParts, fmt.Sprintf("%d other", m.otherJobCount))
	}

	jobsLine := ""
	if len(jobParts) > 0 {
		jobsLine = labelStyle.Render("Jobs: ") + strings.Join(jobParts, ", ")
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		"",
		buildInfo,
		jobsLine,
		"",
	)
}

func (m SummaryModel) renderColumnsSection() string {
	colWidth := (m.width - 10) / 2

	// Left column: Test Results
	testContent := m.renderTestResults()
	leftCol := lipgloss.NewStyle().
		Width(colWidth).
		Render(testContent)

	// Right column: Log Findings
	logContent := m.renderLogFindings()
	rightCol := lipgloss.NewStyle().
		Width(colWidth).
		Render(logContent)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftCol, "    ", rightCol)
}

func (m SummaryModel) renderTestResults() string {
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(m.styles.PrimaryBlue).
		Underline(true)

	labelStyle := lipgloss.NewStyle().
		Foreground(m.styles.TextSecondary)

	var lines []string
	lines = append(lines, headerStyle.Render("Test Results"))
	lines = append(lines, "")

	if !m.hasTestResults {
		lines = append(lines, labelStyle.Render("  No test results found"))
		lines = append(lines, labelStyle.Render("  (build failed before tests)"))
		return lipgloss.JoinVertical(lipgloss.Left, lines...)
	}

	summary := m.testSummary

	// Novel failures
	novelCount := len(summary.NovelFailures)
	if novelCount > 0 {
		novelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
		lines = append(lines, fmt.Sprintf("  %s novel failures", novelStyle.Render(fmt.Sprintf("%d", novelCount))))
	}

	// Flaky failures
	flakyCount := len(summary.FlakyFailures)
	if flakyCount > 0 {
		flakyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500"))
		lines = append(lines, fmt.Sprintf("  %s flaky tests failed", flakyStyle.Render(fmt.Sprintf("%d", flakyCount))))
	}

	// Passed
	if summary.PassedCount > 0 {
		passedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00"))
		lines = append(lines, fmt.Sprintf("  %s passed", passedStyle.Render(fmt.Sprintf("%d", summary.PassedCount))))
	}

	// If no failures
	if novelCount == 0 && flakyCount == 0 && summary.PassedCount > 0 {
		lines = append(lines, labelStyle.Render("  All tests passed!"))
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m SummaryModel) renderLogFindings() string {
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(m.styles.PrimaryBlue).
		Underline(true)

	var lines []string
	lines = append(lines, headerStyle.Render("Log Findings"))
	lines = append(lines, "")

	// Unique errors
	if m.uniqueCount > 0 {
		uniqueStyle := lipgloss.NewStyle().Foreground(m.styles.Tier1Color)
		lines = append(lines, fmt.Sprintf("  %s unique errors", uniqueStyle.Render(fmt.Sprintf("%d", m.uniqueCount))))
	}

	// Noise patterns
	if m.noiseCount > 0 {
		noiseStyle := lipgloss.NewStyle().Foreground(m.styles.Tier3Color)
		lines = append(lines, fmt.Sprintf("  %s noise patterns", noiseStyle.Render(fmt.Sprintf("%d", m.noiseCount))))
	}

	// If no findings
	if m.uniqueCount == 0 && m.noiseCount == 0 {
		labelStyle := lipgloss.NewStyle().Foreground(m.styles.TextSecondary)
		lines = append(lines, labelStyle.Render("  No log findings"))
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m SummaryModel) renderFooter() string {
	footerStyle := lipgloss.NewStyle().
		Foreground(m.styles.TextSecondary).
		Align(lipgloss.Center).
		Width(m.width - 4).
		Padding(1, 0)

	var hints []string
	if m.hasTestResults {
		hints = append(hints, "[t] Tests")
	}
	if m.uniqueCount > 0 || m.noiseCount > 0 {
		hints = append(hints, "[l] Logs")
	}
	hints = append(hints, "[q] Quit")

	return footerStyle.Render("Press " + strings.Join(hints, ", "))
}
