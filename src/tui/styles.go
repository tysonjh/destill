package tui

import "github.com/charmbracelet/lipgloss"

// StyleConfig holds all customizable style colors for the triage UI.
type StyleConfig struct {
	// Primary colors
	PrimaryBlue    lipgloss.Color
	AccentBlue     lipgloss.Color
	AccentYellow   lipgloss.Color
	AccentGreen    lipgloss.Color
	DarkBackground lipgloss.Color
	CardBackground lipgloss.Color
	TextPrimary    lipgloss.Color
	TextSecondary  lipgloss.Color
	BorderColor    lipgloss.Color
	SelectedColor  lipgloss.Color

	// Error highlighting colors
	ErrorForeground lipgloss.Color
	ErrorBackground lipgloss.Color

	// Tier colors for rank display
	Tier1Color lipgloss.Color // Unique failures - red
	Tier3Color lipgloss.Color // Common noise - dim

	// Accent colors for different job types
	JobColors []lipgloss.Color
}

// DefaultStyles returns the default color palette
func DefaultStyles() *StyleConfig {
	return &StyleConfig{
		PrimaryBlue:     lipgloss.Color("#8AB4F8"),
		AccentBlue:      lipgloss.Color("#4285F4"),
		AccentYellow:    lipgloss.Color("#FBBC04"),
		AccentGreen:     lipgloss.Color("#34A853"),
		DarkBackground:  lipgloss.Color("#1E1E1E"),
		CardBackground:  lipgloss.Color("#2D2D2D"),
		TextPrimary:     lipgloss.Color("#E8EAED"),
		TextSecondary:   lipgloss.Color("#9AA0A6"),
		BorderColor:     lipgloss.Color("#5F6368"),
		SelectedColor:   lipgloss.Color("#303134"),
		ErrorForeground: lipgloss.Color("#FF0000"),
		ErrorBackground: lipgloss.Color("#2D0000"),
		Tier1Color:      lipgloss.Color("#FF6B6B"), // Soft red - unique failures
		Tier3Color:      lipgloss.Color("#6B6B6B"), // Dim gray - noise
		JobColors: []lipgloss.Color{
			lipgloss.Color("#34A853"), // Green
			lipgloss.Color("#FBBC04"), // Yellow
			lipgloss.Color("#EA4335"), // Red
			lipgloss.Color("#A142F4"), // Purple
			lipgloss.Color("#24C1E0"), // Cyan
		},
	}
}

// HelpStyle returns a help text lipgloss style using this config
func (s *StyleConfig) HelpStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(s.TextSecondary).
		Padding(0, 2)
}
