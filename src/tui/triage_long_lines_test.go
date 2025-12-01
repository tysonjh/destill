package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"destill-agent/src/contracts"
)

func TestMainModel_LongLineWrapping(t *testing.T) {
	// Create a card with very long log lines that should trigger wrapping
	longLine := "bk;t=[2025-11-30T15:32:45.123Z]%0|1732983165.701|fatal|rdkafka#consumer-1|[thrd:main]:rdkafka#consumer-1:Fatalerror:Local:Brokertransportfailure:ssl://kafka-prod-03.example.com:9093/3:SSLhandshakerror"

	cards := []contracts.TriageCard{
		{
			ID:              "card-1",
			JobName:         "backend",
			Message:         longLine,
			MessageHash:     "abc123",
			ConfidenceScore: 0.95,
			PreContext:      longLine + "\n" + longLine,
			PostContext:     longLine,
		},
	}

	model := createTestModel(cards)

	// Initialize size - 100 width total, 40% = 40 left, 60 right
	updatedModel, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m := updatedModel.(MainModel)

	view := m.View()
	lines := strings.Split(view, "\n")

	// Check that no line exceeds the terminal width
	maxAllowedWidth := 100
	exceededLines := []string{}
	for i, line := range lines {
		// Strip ANSI codes for accurate width measurement
		stripped := stripAnsi(line)
		lineWidth := VisualWidth(stripped)
		if lineWidth > maxAllowedWidth {
			exceededLines = append(exceededLines,
				fmt.Sprintf("line %d: width=%d, content='%s'", i, lineWidth, stripped[:min(50, len(stripped))]))
		}
	}

	if len(exceededLines) > 0 {
		t.Errorf("Found %d lines exceeding terminal width %d:\n%s",
			len(exceededLines), maxAllowedWidth, strings.Join(exceededLines, "\n"))
	}

	// Verify detail viewport was set up with correct width
	// Right panel is 60% = 60, minus border (4) = 56
	// Actually, Width(60) with border includes the border in the total width.
	// So inner content is 60 - 2 = 58.
	expectedViewportWidth := 58
	if m.detailViewport.Width != expectedViewportWidth {
		t.Errorf("viewport width incorrect: expected %d, got %d",
			expectedViewportWidth, m.detailViewport.Width)
	}

	// Check that wrapped content in viewport doesn't exceed its width
	detailContent := m.detailViewport.View()
	detailLines := strings.Split(detailContent, "\n")
	maxDetailWidth := m.detailViewport.Width

	exceededDetailLines := []string{}
	for i, line := range detailLines {
		stripped := stripAnsi(line)
		lineWidth := VisualWidth(stripped)
		if lineWidth > maxDetailWidth {
			exceededDetailLines = append(exceededDetailLines,
				fmt.Sprintf("detail line %d: width=%d, content='%s'", i, lineWidth, stripped[:min(50, len(stripped))]))
		}
	}

	if len(exceededDetailLines) > 0 {
		t.Errorf("Found %d detail lines exceeding viewport width %d:\n%s",
			len(exceededDetailLines), maxDetailWidth, strings.Join(exceededDetailLines, "\n"))
	}
}

// stripAnsi removes ANSI escape codes from a string for accurate width measurement
func stripAnsi(s string) string {
	var result strings.Builder
	inEscape := false
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			inEscape = true
			i++ // skip the '['
			continue
		}
		if inEscape {
			if s[i] == 'm' {
				inEscape = false
			}
			continue
		}
		result.WriteByte(s[i])
	}
	return result.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
