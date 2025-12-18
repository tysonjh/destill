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

	// Title
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(m.styles.PrimaryBlue).
		Width(m.width - 4).
		Align(lipgloss.Center).
		Padding(1, 0)
	title := titleStyle.Render("Test Failures")

	// Content viewport
	content := m.viewport.View()

	// Footer with key bindings
	footerStyle := lipgloss.NewStyle().
		Foreground(m.styles.TextSecondary).
		Align(lipgloss.Center).
		Width(m.width - 4)
	footer := footerStyle.Render("Press [s] Summary, [l] Logs, [q] Quit")

	// Combine
	inner := lipgloss.JoinVertical(lipgloss.Left, title, content, footer)

	// Wrap in a box
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.styles.BorderColor).
		Padding(0, 1).
		Width(m.width - 2)

	return boxStyle.Render(inner)
}

func (m *TestsModel) updateContent() {
	if m.summary == nil {
		m.viewport.SetContent("No test data available")
		return
	}

	var lines []string

	// Summary line
	summaryStyle := lipgloss.NewStyle().
		Foreground(m.styles.TextSecondary)
	totalFailed := len(m.summary.NovelFailures) + len(m.summary.FlakyFailures)
	if totalFailed == 0 {
		lines = append(lines, summaryStyle.Render("All tests passed!"))
		lines = append(lines, "")
	}

	// Novel failures section
	if len(m.summary.NovelFailures) > 0 {
		headerStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(m.styles.Tier1Color)
		lines = append(lines, headerStyle.Render("Novel Failures"))
		lines = append(lines, strings.Repeat("-", 40))

		for _, f := range m.summary.NovelFailures {
			line := m.formatTestFailure(f, false)
			lines = append(lines, line)
		}
		lines = append(lines, "")
	}

	// Flaky failures section
	if len(m.summary.FlakyFailures) > 0 {
		headerStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFA500"))
		lines = append(lines, headerStyle.Render("Known Flaky Tests"))
		lines = append(lines, strings.Repeat("-", 40))

		for _, f := range m.summary.FlakyFailures {
			line := m.formatTestFailure(f, true)
			lines = append(lines, line)
		}
		lines = append(lines, "")
	}

	// If no failures
	if len(m.summary.NovelFailures) == 0 && len(m.summary.FlakyFailures) == 0 {
		if m.summary.TotalTests > 0 {
			passedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00"))
			lines = append(lines, passedStyle.Render(fmt.Sprintf("All %d tests passed!", m.summary.PassedCount)))
		} else {
			lines = append(lines, summaryStyle.Render("No test results available"))
		}
	}

	m.viewport.SetContent(strings.Join(lines, "\n"))
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
