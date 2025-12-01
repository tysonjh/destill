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
	header      Header
	listView    View
	items       []Item
	width       int
	height      int
	styles      *StyleConfig
	searchMode  bool
	searchQuery string
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
		case "/":
			m.searchMode = true
			m.searchQuery = ""
			m.header.SetSearch(m.searchQuery, m.searchMode)
			return m, nil
		}
	}

	// Update list view
	m.listView, cmd = m.listView.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *MainModel) resizeComponents() {
	// Calculate header height dynamically (same as in View)
	headerView := m.header.Render(m.width)
	headerHeight := lipgloss.Height(headerView)
	footerHeight := 1 // Help text

	// Available height for main content
	availableHeight := m.height - headerHeight - footerHeight
	if availableHeight < 10 {
		availableHeight = 10
	}

	// Left panel width (40%)
	leftPanelWidth := int(float64(m.width) * 0.4)

	// Update list size (width minus borders)
	m.listView.SetSize(leftPanelWidth-2, availableHeight)
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
}

func (m MainModel) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	// 1. Header
	headerView := m.header.Render(m.width)
	headerHeight := lipgloss.Height(headerView)

	// Calculate available height for main content
	// Account for: Header + Help Footer (1 line) + some breathing room
	// The border is included in the panel height, so don't double-count it
	const footerHeight = 1
	availableHeight := m.height - headerHeight - footerHeight
	if availableHeight < 10 {
		availableHeight = 10 // Minimum height to show something useful
	}

	// Two-panel layout: Triage List (40%) | Context Detail (60%)
	leftPanelWidth := int(float64(m.width) * 0.4)
	rightPanelWidth := m.width - leftPanelWidth

	// Ensure list size is updated (redundant if resizeComponents called, but safe)
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
	selectedItem, ok := m.listView.GetSelectedItem()

	if ok {
		detailContent := m.renderDetail(selectedItem, rightPanelWidth-4, availableHeight-2)

		// Add placeholder header row with job name
		jobHeaderRow := lipgloss.NewStyle().
			Foreground(m.styles.PrimaryBlue).
			Bold(true).
			Padding(0, 1).
			Render(fmt.Sprintf("Job: %s", selectedItem.Card.JobName))

		detailWithHeader := lipgloss.JoinVertical(lipgloss.Left, jobHeaderRow,
			lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(m.styles.AccentBlue).
				Width(rightPanelWidth).
				Height(availableHeight).
				Render(detailContent))

		rightPanel = detailWithHeader
	} else {
		// Placeholder when nothing selected
		placeholderRow := lipgloss.NewStyle().
			Foreground(m.styles.TextSecondary).
			Padding(0, 1).
			Render(" ")

		emptyStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(m.styles.BorderColor).
			Width(rightPanelWidth).
			Height(availableHeight).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(m.styles.TextSecondary).
			Faint(true)

		rightPanel = lipgloss.JoinVertical(lipgloss.Left, placeholderRow, emptyStyle.Render("← Select a card to view details"))
	}

	// Combine panels horizontally
	mainContent := lipgloss.JoinHorizontal(lipgloss.Top, leftPanelWithHeader, rightPanel)

	// 4. Footer
	helpStyle := m.styles.HelpStyle().Render("↑/↓ navigate • tab filter • q quit")

	// Join vertically
	return lipgloss.JoinVertical(lipgloss.Left,
		headerView,
		mainContent,
		helpStyle,
	)
}

func (m MainModel) renderDetail(item Item, width, height int) string {
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

	// Wrap in viewport style (using strings.Builder content directly)
	// Note: ideally we'd use a real viewport bubble here for scrolling long content,
	// but for now we just render to string.
	return content.String()
}
