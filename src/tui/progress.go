package tui

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ProgressMsg updates progress display
type ProgressMsg struct {
	Stage   string
	Current int
	Total   int
}

type ProgressModel struct {
	stage   string
	current int
	total   int
	done    bool
}

func NewProgressModel() ProgressModel {
	return ProgressModel{}
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
	}
	return m, nil
}

func (m ProgressModel) View() string {
	if m.done {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("✓ Complete!")
	}

	if m.total > 0 {
		pct := float64(m.current) / float64(m.total) * 100
		return fmt.Sprintf("%s (%d/%d, %.0f%%)", m.stage, m.current, m.total, pct)
	}

	return m.stage + "..."
}
