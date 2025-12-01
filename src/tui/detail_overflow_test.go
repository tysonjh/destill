package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"destill-agent/src/contracts"
)

// TestDetailPanel_PlainLongLinesOverflow reproduces the issue where plain long lines
// in the detail view overflow the terminal width and bleed into the list panel,
// causing misalignment issues.
func TestDetailPanel_PlainLongLinesOverflow(t *testing.T) {
	// Create plain long lines without special characters - just long English text
	longLine1 := strings.Repeat("This is a very long line of plain text that should be wrapped properly to avoid overflow ", 5)
	longLine2 := strings.Repeat("Another extremely long line with no special characters just regular words that keep going on and on ", 4)
	longLine3 := strings.Repeat("The quick brown fox jumps over the lazy dog again and again creating a very long line of text ", 6)

	cards := []contracts.TriageCard{
		{
			ID:              "overflow-test",
			JobName:         "backend-service",
			Message:         longLine1,
			MessageHash:     "hash123",
			ConfidenceScore: 0.95,
			Severity:        "HIGH",
			PreContext:      longLine2 + "\n" + longLine3,
			PostContext:     longLine1,
		},
	}

	model := createTestModel(cards)

	// Set terminal width to 100 columns (40% list = 40, 60% detail = 60)
	terminalWidth := 100
	terminalHeight := 30
	updatedModel, _ := model.Update(tea.WindowSizeMsg{Width: terminalWidth, Height: terminalHeight})
	m := updatedModel.(MainModel)

	// Render the complete view
	view := m.View()
	viewLines := strings.Split(view, "\n")

	// Track violations
	var violations []string

	for i, line := range viewLines {
		// Strip ANSI codes to get actual visual content
		stripped := stripAnsi(line)
		visualWidth := VisualWidth(stripped)

		if visualWidth > terminalWidth {
			violations = append(violations, fmt.Sprintf(
				"Line %d exceeds terminal width (%d > %d):\n  First 100 chars: %s...",
				i, visualWidth, terminalWidth, truncateString(stripped, 100)))
		}
	}

	if len(violations) > 0 {
		t.Errorf("Found %d lines overflowing terminal width:\n%s",
			len(violations), strings.Join(violations, "\n"))
	}

	// Additional check: Verify the detail panel content itself doesn't overflow
	// The detail panel should be 60% of 100 = 60 columns
	// With borders, the content area should be <= 58 columns
	expectedDetailWidth := 60
	expectedContentWidth := expectedDetailWidth - 2 // Account for borders

	if m.detailViewport.Width > expectedContentWidth {
		t.Errorf("Detail viewport width (%d) exceeds expected content width (%d)",
			m.detailViewport.Width, expectedContentWidth)
	}

	// Check that wrapped content in the viewport respects the width limit
	detailContent := m.detailViewport.View()
	detailLines := strings.Split(detailContent, "\n")

	var detailViolations []string
	for i, line := range detailLines {
		stripped := stripAnsi(line)
		visualWidth := VisualWidth(stripped)

		if visualWidth > m.detailViewport.Width {
			detailViolations = append(detailViolations, fmt.Sprintf(
				"Detail line %d exceeds viewport width (%d > %d):\n  Content: %s",
				i, visualWidth, m.detailViewport.Width, truncateString(stripped, 80)))
		}
	}

	if len(detailViolations) > 0 {
		t.Errorf("Found %d detail lines overflowing viewport width:\n%s",
			len(detailViolations), strings.Join(detailViolations, "\n"))
	}
}

// TestDetailPanel_VeryLongSingleWord tests that even words longer than the viewport
// width get broken correctly and don't overflow
func TestDetailPanel_VeryLongSingleWord(t *testing.T) {
	// Create a super long "word" (no spaces) that exceeds any reasonable width
	longWord := strings.Repeat("abcdefghijklmnopqrstuvwxyz", 20) // 520 characters

	cards := []contracts.TriageCard{
		{
			ID:              "long-word-test",
			JobName:         "test-job",
			Message:         longWord,
			MessageHash:     "hash456",
			ConfidenceScore: 0.90,
			Severity:        "MEDIUM",
			PreContext:      longWord,
			PostContext:     longWord,
		},
	}

	model := createTestModel(cards)

	terminalWidth := 80
	updatedModel, _ := model.Update(tea.WindowSizeMsg{Width: terminalWidth, Height: 30})
	m := updatedModel.(MainModel)

	view := m.View()
	viewLines := strings.Split(view, "\n")

	var violations []string
	for i, line := range viewLines {
		stripped := stripAnsi(line)
		visualWidth := VisualWidth(stripped)

		if visualWidth > terminalWidth {
			violations = append(violations, fmt.Sprintf(
				"Line %d exceeds terminal width (%d > %d)", i, visualWidth, terminalWidth))
		}
	}

	if len(violations) > 0 {
		t.Errorf("Long word test failed - found %d overflowing lines:\n%s",
			len(violations), strings.Join(violations, "\n"))
	}
}

// TestDetailPanel_MixedContentWithLongLines tests a realistic scenario with
// mixed content including headers, contexts, and long error messages
func TestDetailPanel_MixedContentWithLongLines(t *testing.T) {
	longError := "Error: Failed to connect to database server at hostname-that-is-very-long-for-some-reason.example.com:5432 with credentials user=admin database=production_database timeout=30s retry_count=3 ssl_mode=require connection_pool_size=50"

	preContext := strings.Join([]string{
		"Starting database connection initialization sequence for production environment",
		"Loaded configuration from /etc/myapp/config/production.yaml with overrides from environment variables",
		longError,
	}, "\n")

	postContext := strings.Join([]string{
		"Attempted fallback to secondary database server at backup-hostname-also-quite-long.example.com:5432 but connection failed as well",
		"Rolling back transaction and cleaning up resources allocated during the failed connection attempt",
	}, "\n")

	cards := []contracts.TriageCard{
		{
			ID:              "mixed-content-test",
			JobName:         "database-init",
			Message:         longError,
			MessageHash:     "hash789",
			ConfidenceScore: 0.88,
			Severity:        "CRITICAL",
			PreContext:      preContext,
			PostContext:     postContext,
		},
	}

	model := createTestModel(cards)

	// Test with a narrower terminal to make overflow more likely
	terminalWidth := 80
	updatedModel, _ := model.Update(tea.WindowSizeMsg{Width: terminalWidth, Height: 40})
	m := updatedModel.(MainModel)

	view := m.View()
	viewLines := strings.Split(view, "\n")

	var violations []string
	for i, line := range viewLines {
		stripped := stripAnsi(line)
		visualWidth := VisualWidth(stripped)

		if visualWidth > terminalWidth {
			violations = append(violations, fmt.Sprintf(
				"Line %d: width=%d, preview=%s",
				i, visualWidth, truncateString(stripped, 60)))
		}
	}

	if len(violations) > 0 {
		t.Errorf("Mixed content test failed - %d lines overflow terminal width %d:\n%s",
			len(violations), terminalWidth, strings.Join(violations, "\n"))
	}
}

// Helper function to truncate a string for display in error messages
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
