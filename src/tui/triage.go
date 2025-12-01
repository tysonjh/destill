package tui

import (
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

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
	ready          bool
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
		ready:          false,
	}

	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// Init initializes the model
func (m MainModel) Init() tea.Cmd {
	return nil
}

// Update handles messages and updates the model
func (m MainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		if !m.ready {
			// Initialize viewport with calculated dimensions
			dims := m.calculateDimensions()
			m.detailViewport = viewport.New(dims.rightPanelWidth-2, dims.availableHeight-1)
			m.ready = true
		}

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
