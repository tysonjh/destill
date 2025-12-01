package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"destill-agent/src/contracts"
)

// MainModel is the main Bubble Tea model for the application.
type MainModel struct {
	header         Header
	listView       View
	items          []Item
	detailViewport viewport.Model
	detailFocused  bool
	width          int
	height         int
	styles         *StyleConfig
	searchMode     bool
	searchQuery    string
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
		header:         header,
		listView:       listView,
		items:          items,
		styles:         styles,
		detailViewport: viewport.New(0, 0),
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

		// Initialize detail content with first item on first render
		if selectedItem, ok := m.listView.GetSelectedItem(); ok {
			m.updateDetailContent(selectedItem)
		}

	case tea.KeyMsg:
		// Handle search mode input
		if m.searchMode {
			switch msg.String() {
			case "esc":
				m.searchMode = false
				m.searchQuery = ""
				m.header.SetSearch(m.searchQuery, m.searchMode)
				m.applyFilter()
				return m, nil
			case "enter":
				m.searchMode = false
				m.header.SetSearch(m.searchQuery, m.searchMode)
				m.applyFilter()
				return m, nil
			case "backspace":
				if len(m.searchQuery) > 0 {
					m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
					m.header.SetSearch(m.searchQuery, m.searchMode)
					m.applyFilter()
				}
				return m, nil
			default:
				// Add character to search query if it's a single rune
				if len(msg.String()) == 1 {
					m.searchQuery += msg.String()
					m.header.SetSearch(m.searchQuery, m.searchMode)
					m.applyFilter()
				}
				return m, nil
			}
		}

		// Standard navigation
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			m.header.CycleFilter()
			m.applyFilter()
			return m, nil
		case "shift+tab":
			m.header.CycleFilterBackward()
			m.applyFilter()
			return m, nil
		case "/":
			m.searchMode = true
			m.searchQuery = ""
			m.header.SetSearch(m.searchQuery, m.searchMode)
			return m, nil
		case "enter":
			// Toggle focus to detail viewport
			m.detailFocused = !m.detailFocused
			return m, nil
		case "esc":
			// If detail is focused, return to list
			if m.detailFocused {
				m.detailFocused = false
				return m, nil
			}
			// Otherwise reset filter to ALL
			m.header.ResetFilter()
			m.applyFilter()
			return m, nil
		}
	}

	// Route updates based on focus
	if m.detailFocused {
		// Detail viewport is focused, send keys to it
		m.detailViewport, cmd = m.detailViewport.Update(msg)
		cmds = append(cmds, cmd)
	} else {
		// List is focused, send keys to it
		m.listView, cmd = m.listView.Update(msg)
		cmds = append(cmds, cmd)
		// Update detail content when list selection changes
		if selectedItem, ok := m.listView.GetSelectedItem(); ok {
			m.updateDetailContent(selectedItem)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *MainModel) resizeComponents() {
	// Calculate list size
	leftPanelWidth := int(float64(m.width) * 0.4)
	rightPanelWidth := m.width - leftPanelWidth
	availableHeight := m.height - 7 // header(3) + help(1) + borders(2) + padding(1)
	m.listView.SetSize(leftPanelWidth-4, availableHeight)

	// Resize viewport for detail panel (accounting for borders and job header)
	m.detailViewport.Width = rightPanelWidth - 4
	m.detailViewport.Height = availableHeight - 4
}

func (m *MainModel) applyFilter() {
	filter := m.header.GetFilter()

	// 1. Filter by Job
	var filtered []Item
	if filter == "ALL" {
		filtered = m.items
	} else {
		for _, item := range m.items {
			if item.Card.JobName == filter {
				filtered = append(filtered, item)
			}
		}
	}

	// 2. Filter by Search Query
	if m.searchQuery != "" {
		var searchFiltered []Item
		query := strings.ToLower(m.searchQuery)
		for _, item := range filtered {
			// Search in message, job name, hash, severity
			if strings.Contains(strings.ToLower(item.Card.Message), query) ||
				strings.Contains(strings.ToLower(item.Card.JobName), query) ||
				strings.Contains(strings.ToLower(item.Card.MessageHash), query) ||
				strings.Contains(strings.ToLower(item.Card.Severity), query) {
				searchFiltered = append(searchFiltered, item)
				continue
			}
			// Search in PreContext
			for _, line := range item.GetPreContext() {
				if strings.Contains(strings.ToLower(line), query) {
					searchFiltered = append(searchFiltered, item)
					goto nextItem
				}
			}
			// Search in PostContext
			for _, line := range item.GetPostContext() {
				if strings.Contains(strings.ToLower(line), query) {
					searchFiltered = append(searchFiltered, item)
					goto nextItem
				}
			}
		nextItem:
		}
		filtered = searchFiltered
	}

	m.listView.SetItems(filtered)
	// Update detail content for new selection
	if selectedItem, ok := m.listView.GetSelectedItem(); ok {
		m.updateDetailContent(selectedItem)
	}
}

func (m MainModel) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	// Render header (now includes search)
	header := m.header.Render(m.width)
	headerHeight := lipgloss.Height(header)

	// Calculate available height more carefully
	// Account for: header + help line + panel borders (2 lines per panel)
	availableHeight := m.height - headerHeight - 2 - 2

	// Two-panel layout: Triage List (40%) | Context Detail (60%)
	leftPanelWidth := int(float64(m.width) * 0.4)
	rightPanelWidth := m.width - leftPanelWidth

	// Set list size (accounting for panel borders)
	m.listView.SetSize(leftPanelWidth-2, availableHeight)

	// Left panel: Triage list
	leftPanel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.styles.BorderColor).
		Width(leftPanelWidth).
		Height(availableHeight).
		Render(m.listView.Render())

	// Add column headers to left panel
	delegate := m.listView.GetDelegate()
	rankHeader := fmt.Sprintf("%*s", delegate.RankWidth, "Rk")
	recurHeader := fmt.Sprintf("%*s", delegate.RecurWidth, "Rc")

	headerRow := lipgloss.NewStyle().
		Foreground(m.styles.PrimaryBlue).
		Bold(true).
		Padding(0, 1).
		Render(fmt.Sprintf("%s │ Conf │ %s │ Hash  │ Message", rankHeader, recurHeader))

	leftPanelWithHeader := lipgloss.JoinVertical(lipgloss.Left, headerRow, leftPanel)

	// Right panel: Context detail view
	var rightPanel string
	if selectedItem, ok := m.listView.GetSelectedItem(); ok {
		// Add placeholder header row with job name
		placeholderRow := lipgloss.NewStyle().
			Foreground(m.styles.PrimaryBlue).
			Bold(true).
			Padding(0, 1).
			Render(fmt.Sprintf("Job: %s", selectedItem.Card.JobName))

		// Show viewport content
		borderStyle := m.styles.BorderColor
		if m.detailFocused {
			borderStyle = m.styles.AccentBlue
		}

		detailWithHeader := lipgloss.JoinVertical(lipgloss.Left, placeholderRow,
			lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(borderStyle).
				Width(rightPanelWidth).
				Height(availableHeight). // Match left panel height
				Render(m.detailViewport.View()))

		rightPanel = detailWithHeader
	} else {
		// Add placeholder header row to align with left panel
		placeholderRow := lipgloss.NewStyle().
			Foreground(m.styles.TextSecondary).
			Padding(0, 1).
			Render(" ") // Empty for now

		emptyStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(m.styles.BorderColor).
			Width(rightPanelWidth).
			Height(availableHeight). // Match left panel height
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(m.styles.TextSecondary).
			Faint(true)

		rightPanel = lipgloss.JoinVertical(lipgloss.Left, placeholderRow, emptyStyle.Render("← Navigate list to view details"))
	}

	// Combine panels horizontally
	mainContent := lipgloss.JoinHorizontal(lipgloss.Top, leftPanelWithHeader, rightPanel)

	// Build help text with highlighted key bindings
	keyStyle := lipgloss.NewStyle().Foreground(m.styles.PrimaryBlue).Bold(true)
	sepStyle := lipgloss.NewStyle().Foreground(m.styles.TextSecondary)

	var helpText string
	if m.detailFocused {
		helpText = fmt.Sprintf("%s: Scroll %s %s: Back %s %s: Quit",
			keyStyle.Render("j/k"), sepStyle.Render("•"),
			keyStyle.Render("Esc"), sepStyle.Render("•"),
			keyStyle.Render("q"))
	} else {
		helpText = fmt.Sprintf("%s: Nav %s %s: View %s %s: Job %s %s: Search %s %s: Reset %s %s: Quit",
			keyStyle.Render("j/k"), sepStyle.Render("•"),
			keyStyle.Render("Enter"), sepStyle.Render("•"),
			keyStyle.Render("Tab"), sepStyle.Render("•"),
			keyStyle.Render("/"), sepStyle.Render("•"),
			keyStyle.Render("Esc"), sepStyle.Render("•"),
			keyStyle.Render("q"))
	}
	help := m.styles.HelpStyle().Render(helpText)

	return lipgloss.JoinVertical(lipgloss.Left, header, mainContent, help)
}

func (m MainModel) renderDetail(item Item) string {
	content := strings.Builder{}

	// Detail Header
	header := lipgloss.NewStyle().
		Foreground(m.styles.PrimaryBlue).
		Bold(true).
		Render(fmt.Sprintf("Hash: %s | Severity: %s | Job: %s",
			item.Card.MessageHash,
			item.Card.Severity,
			item.Card.JobName))
	fmt.Fprintf(&content, "%s\n\n", header)

	// Pre-context
	preContext := item.GetPreContext()
	if len(preContext) > 0 {
		fmt.Fprintln(&content, lipgloss.NewStyle().Foreground(m.styles.TextSecondary).Bold(true).Render("Pre-Context:"))
		for _, line := range preContext {
			if strings.TrimSpace(line) != "" {
				fmt.Fprintln(&content, lipgloss.NewStyle().Foreground(m.styles.TextSecondary).Faint(true).Render(line))
			}
		}
		fmt.Fprintln(&content, "")
	}

	// Error Message (Highlight)
	fmt.Fprintln(&content, lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Bold(true).Render("ERROR:"))
	fmt.Fprintln(&content, lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FF0000")).
		Background(lipgloss.Color("#2D0000")).
		Render(item.Card.Message))
	fmt.Fprintln(&content, "")

	// Post-context
	postContext := item.GetPostContext()
	if len(postContext) > 0 {
		fmt.Fprintln(&content, lipgloss.NewStyle().Foreground(m.styles.TextSecondary).Bold(true).Render("Post-Context:"))
		for _, line := range postContext {
			if strings.TrimSpace(line) != "" {
				fmt.Fprintln(&content, lipgloss.NewStyle().Foreground(m.styles.TextSecondary).Faint(true).Render(line))
			}
		}
	}

	return content.String()
}

// updateDetailContent updates the viewport with content from the selected item
func (m *MainModel) updateDetailContent(item Item) {
	content := m.renderDetail(item)
	m.detailViewport.SetContent(content)
}
