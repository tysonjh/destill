package tui

import "github.com/charmbracelet/lipgloss"

// StyleConfig holds all customizable style colors for the triage UI.
type StyleConfig struct {
	// Primary colors
	PrimaryBlue    lipgloss.Color
	AccentBlue     lipgloss.Color
	DarkBackground lipgloss.Color
	CardBackground lipgloss.Color
	TextPrimary    lipgloss.Color
	TextSecondary  lipgloss.Color
	BorderColor    lipgloss.Color
	SelectedColor  lipgloss.Color

	// Accent colors for different job types
	JobColors []lipgloss.Color
}

// DefaultStyles returns the default color palette
func DefaultStyles() *StyleConfig {
	return &StyleConfig{
		PrimaryBlue:    lipgloss.Color("#8AB4F8"),
		AccentBlue:     lipgloss.Color("#4285F4"),
		DarkBackground: lipgloss.Color("#1E1E1E"),
		CardBackground: lipgloss.Color("#2D2D2D"),
		TextPrimary:    lipgloss.Color("#E8EAED"),
		TextSecondary:  lipgloss.Color("#9AA0A6"),
		BorderColor:    lipgloss.Color("#5F6368"),
		SelectedColor:  lipgloss.Color("#303134"),
		JobColors: []lipgloss.Color{
			lipgloss.Color("#34A853"), // Green
			lipgloss.Color("#FBBC04"), // Yellow
			lipgloss.Color("#EA4335"), // Red
			lipgloss.Color("#A142F4"), // Purple
			lipgloss.Color("#24C1E0"), // Cyan
		},
	}
}

// BaseStyle returns a base lipgloss style using this config
func (s *StyleConfig) BaseStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Background(s.DarkBackground).
		Foreground(s.TextPrimary)
}

// TitleStyle returns a title lipgloss style using this config
func (s *StyleConfig) TitleStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(s.PrimaryBlue).
		Bold(true).
		Padding(0, 1)
}

// HelpStyle returns a help text lipgloss style using this config
func (s *StyleConfig) HelpStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(s.TextSecondary).
		Padding(0, 2)
}

// ListStyle returns a list container lipgloss style using this config
func (s *StyleConfig) ListStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Background(s.CardBackground).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(s.BorderColor)
}

// ViewportStyle returns a viewport container lipgloss style using this config
func (s *StyleConfig) ViewportStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Background(s.CardBackground).
		Foreground(s.TextPrimary).
		Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(s.BorderColor)
}
