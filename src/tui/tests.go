package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"destill-agent/src/contracts"
)

// TestsModel displays test failures with flaky indicators.
type TestsModel struct {
	width      int
	height     int
	styles     *StyleConfig
	viewport   viewport.Model
	ready      bool
	summary    *contracts.TestSummary
	allResults []contracts.TestResult
}

// NewTestsModel creates a new tests view model.
func NewTestsModel(styles *StyleConfig) TestsModel {
	return TestsModel{
		styles: styles,
	}
}

// SetTestData updates the test data.
func (m *TestsModel) SetTestData(summary *contracts.TestSummary, results []contracts.TestResult) {
	m.summary = summary
	m.allResults = results
	if m.ready {
		m.updateContent()
	}
}

// SetSize sets the view dimensions.
func (m *TestsModel) SetSize(width, height int) {
	m.width = width
	m.height = height

	if !m.ready {
		m.viewport = viewport.New(width-4, height-6)
		m.ready = true
	} else {
		m.viewport.Width = width - 4
		m.viewport.Height = height - 6
	}
	m.updateContent()
}

// Update handles messages.
func (m TestsModel) Update(msg tea.Msg) (TestsModel, tea.Cmd) {
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// View renders the tests view.
func (m TestsModel) View() string {
	if m.width == 0 {
		return ""
	}

	// Title with scroll indicator
	titleText := "Test Failures"
	if m.viewport.TotalLineCount() > m.viewport.Height {
		scrollPct := int(m.viewport.ScrollPercent() * 100)
		titleText = fmt.Sprintf("Test Failures [%d%%]", scrollPct)
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(m.styles.PrimaryBlue).
		Width(m.width - 4).
		Align(lipgloss.Center).
		Padding(1, 0)
	title := titleStyle.Render(titleText)

	// Content viewport
	content := m.viewport.View()

	// Combine (no footer - main layout provides it)
	inner := lipgloss.JoinVertical(lipgloss.Left, title, content)

	// Wrap in a box
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.styles.BorderColor).
		Padding(0, 1).
		Width(m.width - 2)

	return boxStyle.Render(inner)
}

// dedupedFailure represents a test failure deduplicated across jobs
type dedupedFailure struct {
	TestName       string
	FailureMessage string
	Jobs           []string
	IsFlaky        bool
	FailureRate    float64
}

func (m *TestsModel) updateContent() {
	if m.summary == nil {
		m.viewport.SetContent("No test data available")
		return
	}

	// Deduplicate failures by test name, tracking which jobs they occurred in
	deduped := m.deduplicateFailures()

	var lines []string

	// Summary counts at the top (using raw counts from TestSummary)
	novelCount := len(m.summary.NovelFailures)
	flakyCount := len(m.summary.FlakyFailures)

	summaryStyle := lipgloss.NewStyle().Foreground(m.styles.TextSecondary)
	novelStyle := lipgloss.NewStyle().Bold(true).Foreground(m.styles.Tier1Color)
	flakyStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFA500"))
	passedStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00FF00"))

	summaryLine := fmt.Sprintf("%s novel  %s flaky  %s passed  (%d total)",
		novelStyle.Render(fmt.Sprintf("%d", novelCount)),
		flakyStyle.Render(fmt.Sprintf("%d", flakyCount)),
		passedStyle.Render(fmt.Sprintf("%d", m.summary.PassedCount)),
		m.summary.TotalTests,
	)
	lines = append(lines, summaryLine)
	lines = append(lines, "")

	if len(deduped) == 0 {
		if m.summary.TotalTests > 0 {
			lines = append(lines, passedStyle.Render(fmt.Sprintf("All %d tests passed!", m.summary.PassedCount)))
		} else {
			lines = append(lines, summaryStyle.Render("No test results available"))
		}
		m.viewport.SetContent(strings.Join(lines, "\n"))
		return
	}

	// Separate novel and flaky
	var novelFailures, flakyFailures []dedupedFailure
	for _, d := range deduped {
		if d.IsFlaky {
			flakyFailures = append(flakyFailures, d)
		} else {
			novelFailures = append(novelFailures, d)
		}
	}

	// Novel failures section
	if len(novelFailures) > 0 {
		headerStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(m.styles.Tier1Color)
		lines = append(lines, headerStyle.Render(fmt.Sprintf("Novel Failures (%d unique tests)", len(novelFailures))))
		lines = append(lines, strings.Repeat("-", 40))

		for _, f := range novelFailures {
			lines = append(lines, m.formatDedupedFailure(f)...)
		}
		lines = append(lines, "")
	}

	// Flaky failures section
	if len(flakyFailures) > 0 {
		headerStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFA500"))
		lines = append(lines, headerStyle.Render(fmt.Sprintf("Known Flaky Tests (%d unique tests)", len(flakyFailures))))
		lines = append(lines, strings.Repeat("-", 40))

		for _, f := range flakyFailures {
			lines = append(lines, m.formatDedupedFailure(f)...)
		}
		lines = append(lines, "")
	}

	m.viewport.SetContent(strings.Join(lines, "\n"))
}

// deduplicateFailures groups test failures by test name and collects job info
func (m *TestsModel) deduplicateFailures() []dedupedFailure {
	// Map test name -> dedupedFailure
	byName := make(map[string]*dedupedFailure)

	for _, result := range m.allResults {
		if result.Passed {
			continue
		}

		if existing, ok := byName[result.TestName]; ok {
			// Add job if not already present
			found := false
			for _, j := range existing.Jobs {
				if j == result.JobName {
					found = true
					break
				}
			}
			if !found {
				existing.Jobs = append(existing.Jobs, result.JobName)
			}
		} else {
			byName[result.TestName] = &dedupedFailure{
				TestName:       result.TestName,
				FailureMessage: result.FailureMessage,
				Jobs:           []string{result.JobName},
				IsFlaky:        false, // TODO: integrate with flaky detection
				FailureRate:    0,
			}
		}
	}

	// Convert to slice
	var results []dedupedFailure
	for _, d := range byName {
		results = append(results, *d)
	}

	return results
}

// formatDedupedFailure formats a deduplicated failure with job count
func (m *TestsModel) formatDedupedFailure(f dedupedFailure) []string {
	var lines []string

	// Label style
	var labelStyle lipgloss.Style
	var label string
	if f.IsFlaky {
		labelStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFA500"))
		label = "[FLAKY]"
	} else {
		labelStyle = lipgloss.NewStyle().Bold(true).Foreground(m.styles.Tier1Color)
		label = "[NOVEL]"
	}

	nameStyle := lipgloss.NewStyle().Foreground(m.styles.TextPrimary)
	jobStyle := lipgloss.NewStyle().Foreground(m.styles.TextSecondary)

	// Truncate test name if too long
	testName := f.TestName
	maxNameLen := m.width - 40
	if maxNameLen > 10 && len(testName) > maxNameLen {
		testName = "..." + testName[len(testName)-maxNameLen+3:]
	}

	// Main line: [NOVEL] TestName (failed in X jobs)
	jobInfo := fmt.Sprintf("(failed in %d job", len(f.Jobs))
	if len(f.Jobs) != 1 {
		jobInfo += "s"
	}
	jobInfo += ")"

	mainLine := fmt.Sprintf("%s %s %s",
		labelStyle.Render(label),
		nameStyle.Render(testName),
		jobStyle.Render(jobInfo),
	)
	lines = append(lines, mainLine)

	// Job names on next line, indented
	if len(f.Jobs) > 0 {
		jobNames := strings.Join(f.Jobs, ", ")
		if len(jobNames) > m.width-10 {
			jobNames = jobNames[:m.width-13] + "..."
		}
		lines = append(lines, jobStyle.Render("    └─ "+jobNames))
	}

	return lines
}

func (m *TestsModel) formatTestFailure(f contracts.TestFailure, isFlaky bool) string {
	// Format: [NOVEL] or [FLAKY] + test name + failure rate
	var labelStyle lipgloss.Style
	var label string

	if isFlaky {
		labelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFA500"))
		label = "[FLAKY]"
	} else {
		labelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(m.styles.Tier1Color)
		label = "[NOVEL]"
	}

	nameStyle := lipgloss.NewStyle().
		Foreground(m.styles.TextPrimary)

	rateStyle := lipgloss.NewStyle().
		Foreground(m.styles.TextSecondary)

	// Calculate rate string
	rateStr := ""
	if f.FailureRate > 0 {
		// Estimate failures out of 20 runs
		failed := int(f.FailureRate*20 + 0.5)
		rateStr = fmt.Sprintf("(%d/20 failures)", failed)
	} else if !isFlaky {
		rateStr = "(first failure)"
	}

	// Truncate test name if too long
	testName := f.TestName
	maxNameLen := m.width - 30
	if maxNameLen > 10 && len(testName) > maxNameLen {
		testName = "..." + testName[len(testName)-maxNameLen+3:]
	}

	return fmt.Sprintf("%s %s %s",
		labelStyle.Render(label),
		nameStyle.Render(testName),
		rateStyle.Render(rateStr),
	)
}

// HasTestFailures returns true if there are any test failures.
func (m *TestsModel) HasTestFailures() bool {
	if m.summary == nil {
		return false
	}
	return len(m.summary.NovelFailures) > 0 || len(m.summary.FlakyFailures) > 0
}

// HasTests returns true if there are any test results.
func (m *TestsModel) HasTests() bool {
	return m.summary != nil && m.summary.TotalTests > 0
}
