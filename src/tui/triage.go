package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"destill-agent/src/contracts"
)

// MainModel is the main Bubble Tea model for the application.
type MainModel struct {
	header       Header
	listView     View
	items        []Item
	width        int
	height       int
	styles       *StyleConfig
}

// Start initializes and runs the TUI with the provided triage cards.
func Start(cards []contracts.TriageCard) error {
	// Convert cards to Items
	items := make([]Item, len(cards))
	jobSet := make(map[string]bool)
	var jobs []string

	for i, card := range cards {
		items[i] = Item{
			Card: card,
			Rank: i + 1,
		}
		if !jobSet[card.JobName] {
			jobSet[card.JobName] = true
			jobs = append(jobs, card.JobName)
		}
	}

	// Initialize styles
	styles := DefaultStyles()

	// Initialize header
	header := NewHeaderWithStyles("Destill Analysis", jobs, styles)

	// Initialize list view
	listView := NewView()
	listView.SetItems(items)

	model := MainModel{
		header:   header,
		listView: listView,
		items:    items,
		styles:   styles,
	}

	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m MainModel) Init() tea.Cmd {
	return nil
}

func (m MainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeComponents()

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			m.header.CycleFilter()
			m.applyFilter()
		}
	}

	// Update list view
	m.listView, cmd = m.listView.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *MainModel) resizeComponents() {
	// Calculate heights
	headerHeight := 3 // Approx height of header (border + padding)
	footerHeight := 1 // Help text
	availableHeight := m.height - headerHeight - footerHeight

	// Split remaining height: 1/3 for list, 2/3 for details
	listHeight := availableHeight / 3
	if listHeight < 5 {
		listHeight = 5
	}
	
m.listView.SetSize(m.width, listHeight)
}

func (m *MainModel) applyFilter() {
	filter := m.header.GetFilter()
	if filter == "ALL" {
		m.listView.SetItems(m.items)
		return
	}

	var filtered []Item
	for _, item := range m.items {
		if item.Card.JobName == filter {
			filtered = append(filtered, item)
		}
	}
	m.listView.SetItems(filtered)
}

func (m MainModel) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	// 1. Header
	headerView := m.header.Render(m.width)

	// 2. List
	listView := m.listView.Render()

	// 3. Detail View
	detailView := m.renderDetail()

	// 4. Footer
	helpStyle := m.styles.HelpStyle()
	footerView := helpStyle.Render("↑/↓ navigate • tab filter • q quit")

	// Join vertically
	return lipgloss.JoinVertical(lipgloss.Left,
		headerView,
		listView,
		detailView,
		footerView,
	)
}

func (m MainModel) renderDetail() string {
	selectedItem, ok := m.listView.GetSelectedItem()
	if !ok {
		return ""
	}

	// Calculate detail height
	headerHeight := lipgloss.Height(m.header.Render(m.width))
	listHeight := lipgloss.Height(m.listView.Render())
	footerHeight := 1
	detailHeight := m.height - headerHeight - listHeight - footerHeight

	if detailHeight < 0 {
		detailHeight = 0
	}

	content := strings.Builder{}

	// Detail Header
	titleStyle := m.styles.TitleStyle()
	fmt.Fprintf(&content, "%s\n\n", titleStyle.Render("Failure Details"))

	// Job Name
	jobStyle := lipgloss.NewStyle().Bold(true).Foreground(m.styles.PrimaryBlue)
	fmt.Fprintf(&content, "Job: %s\n\n", jobStyle.Render(selectedItem.Card.JobName))

	// Context
	contextStyle := lipgloss.NewStyle().Foreground(m.styles.TextSecondary)
	
	// Pre-context
	for _, line := range selectedItem.GetPreContext() {
		if strings.TrimSpace(line) != "" {
			fmt.Fprintln(&content, contextStyle.Render(line))
		}
	}

	// Error Message (Highlight)
	errorStyle := lipgloss.NewStyle().
		Foreground(m.styles.AccentBlue).
		Bold(true)
	fmt.Fprintln(&content, errorStyle.Render(selectedItem.Card.Message))

	// Post-context
	for _, line := range selectedItem.GetPostContext() {
		if strings.TrimSpace(line) != "" {
			fmt.Fprintln(&content, contextStyle.Render(line))
		}
	}

	// Wrap in viewport style
	viewportStyle := m.styles.ViewportStyle().
		Width(m.width - 4). // Account for padding/borders
		Height(detailHeight)

	return viewportStyle.Render(content.String())
}