package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ASCII art logo lines for loading screen
var destillLogo = []string{
	" ███████▄  ██████████ ▄████████▄ ████████ ████ ██       ██",
	" ██    ██  ██         ██             ██    ██  ██       ██",
	" ██     █  ██████      ████████      ██    ██  ██       ██",
	" ██    ██  ██                 ██     ██    ██  ██       ██",
	"       ███████▀  ██████████ ▀████████▀     ██   ████ ████████ ████████",
}

// Gradient colors from light (top) to dark (bottom) - subtle variation
var logoGradientColors = []string{
	"#5DADE2", // Slightly lighter (top)
	"#3498DB", // Base blue
	"#2E86C1", // Slightly darker
	"#2874A6", // A bit darker
	"#21618C", // Darkest (bottom)
}

// Spinner frames for retro loading animation
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// ProgressMsg updates progress display
type ProgressMsg struct {
	Stage   string
	Current int
	Total   int
}

// SpinnerTickMsg triggers spinner animation frame advance
type SpinnerTickMsg time.Time

type ProgressModel struct {
	stage        string
	current      int
	total        int
	done         bool
	spinnerFrame int
}

func NewProgressModel() ProgressModel {
	return ProgressModel{spinnerFrame: 0}
}

// SpinnerTick returns a command that sends SpinnerTickMsg after a delay
func SpinnerTick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg {
		return SpinnerTickMsg(t)
	})
}

func (m ProgressModel) Update(msg tea.Msg) (ProgressModel, tea.Cmd) {
	switch msg := msg.(type) {
	case ProgressMsg:
		m.stage = msg.Stage
		m.current = msg.Current
		m.total = msg.Total
		if msg.Stage == "complete" {
			m.done = true
		}
	case SpinnerTickMsg:
		m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
		if !m.done {
			return m, SpinnerTick()
		}
	}
	return m, nil
}

func (m ProgressModel) View() string {
	// Render logo with gradient colors (each line gets a different color)
	var logoLines []string
	for i, line := range destillLogo {
		color := logoGradientColors[i%len(logoGradientColors)]
		style := lipgloss.NewStyle().
			Foreground(lipgloss.Color(color)).
			Bold(true)
		logoLines = append(logoLines, style.Render(line))
	}
	logo := strings.Join(logoLines, "\n")

	if m.done {
		completeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
		status := completeStyle.Render("✓ Complete! Press (r) to refresh")
		return lipgloss.JoinVertical(lipgloss.Center, logo, "", status)
	}

	// Build progress line with spinner
	spinner := spinnerFrames[m.spinnerFrame]
	spinnerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD700")) // Gold

	var statusLine string
	if m.total > 0 {
		pct := float64(m.current) / float64(m.total) * 100
		statusLine = fmt.Sprintf("%s %s (%d/%d, %.0f%%)",
			spinnerStyle.Render(spinner), m.stage, m.current, m.total, pct)
	} else if m.stage != "" {
		statusLine = fmt.Sprintf("%s %s...", spinnerStyle.Render(spinner), m.stage)
	} else {
		statusLine = fmt.Sprintf("%s Loading...", spinnerStyle.Render(spinner))
	}

	return lipgloss.JoinVertical(lipgloss.Center, logo, "", statusLine)
}
