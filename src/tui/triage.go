package tui

import (
	"encoding/json"
	"sort"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"destill-agent/src/contracts"
)

// LoadStatus represents the current loading state of the TUI
type LoadStatus int

const (
	StatusLoading  LoadStatus = iota // Still fetching data
	StatusComplete                   // All data loaded
	StatusError                      // Error occurred
)

// cardReceivedMsg is sent when a new triage card arrives from the broker
type cardReceivedMsg struct {
	card contracts.TriageCard
}

// pipelineCompleteMsg is sent when the pipeline signals completion
type pipelineCompleteMsg struct{}

// pipelineErrorMsg is sent when there's an error in the pipeline
type pipelineErrorMsg struct {
	err error
}

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

	// Streaming support
	broker        contracts.MessageBroker
	cardChan      <-chan []byte      // Channel receiving cards from broker
	pendingCards  []Item             // Cards waiting to be merged
	hashMap       map[string]*Item   // For grouping by hash
	status        LoadStatus         // Current loading status
	cardCount     int                // Total cards received
	jobsDiscovered map[string]bool   // Jobs we've seen so far
}

// Start initializes and runs the TUI with the provided triage cards (legacy mode).
func Start(cards []contracts.TriageCard) error {
	return StartWithBroker(nil, cards)
}

// StartWithBroker initializes the TUI in streaming mode with a message broker.
// If broker is nil, uses the provided initial cards only (legacy mode).
// If broker is provided, subscribes to ci_failures_ranked for live updates.
func StartWithBroker(broker contracts.MessageBroker, initialCards []contracts.TriageCard) error {
	// Initialize styles
	styles := DefaultStyles()

	// Build initial state from provided cards
	hashMap := make(map[string]*Item)
	jobsDiscovered := make(map[string]bool)
	var jobs []string

	for _, card := range initialCards {
		if existing, ok := hashMap[card.MessageHash]; ok {
			// Increment recurrence count
			count := existing.GetRecurrence() + 1
			existing.Card.Metadata["recurrence_count"] = string(rune('0' + count))
		} else {
			item := Item{Card: card, Rank: 0}
			hashMap[card.MessageHash] = &item
		}
		if !jobsDiscovered[card.JobName] {
			jobsDiscovered[card.JobName] = true
			jobs = append(jobs, card.JobName)
		}
	}

	// Convert map to sorted slice
	items := hashMapToSortedItems(hashMap)

	// Determine initial status
	status := StatusComplete
	if broker != nil {
		status = StatusLoading
	}

	// Initialize header
	header := NewHeaderWithStyles("Destill Analysis", jobs, styles)
	header.SetLoadStatus(status, len(items), len(jobsDiscovered))

	// Initialize list view
	listView := NewView()
	listView.SetItems(items)

	// Subscribe to broker if provided
	var cardChan <-chan []byte
	if broker != nil {
		var err error
		cardChan, err = broker.Subscribe("ci_failures_ranked")
		if err != nil {
			return err
		}
	}

	model := MainModel{
		header:          header,
		listView:        listView,
		items:           items,
		styles:          styles,
		detailViewport:  viewport.New(0, 0),
		ready:           false,
		broker:          broker,
		cardChan:        cardChan,
		pendingCards:    nil,
		hashMap:         hashMap,
		status:          status,
		cardCount:       len(initialCards),
		jobsDiscovered:  jobsDiscovered,
	}

	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// hashMapToSortedItems converts the hash map to a sorted slice of items
func hashMapToSortedItems(hashMap map[string]*Item) []Item {
	items := make([]Item, 0, len(hashMap))
	for _, item := range hashMap {
		items = append(items, *item)
	}

	// Sort by confidence (desc), then recurrence (desc)
	sort.Slice(items, func(i, j int) bool {
		if items[i].Card.ConfidenceScore != items[j].Card.ConfidenceScore {
			return items[i].Card.ConfidenceScore > items[j].Card.ConfidenceScore
		}
		return items[i].GetRecurrence() > items[j].GetRecurrence()
	})

	// Assign ranks
	for i := range items {
		items[i].Rank = i + 1
	}

	return items
}

// Init initializes the model
func (m MainModel) Init() tea.Cmd {
	if m.cardChan != nil {
		// Start listening for cards from broker
		return listenForCards(m.cardChan)
	}
	return nil
}

// listenForCards returns a command that waits for the next card from the broker
func listenForCards(cardChan <-chan []byte) tea.Cmd {
	return func() tea.Msg {
		data, ok := <-cardChan
		if !ok {
			// Channel closed, pipeline complete
			return pipelineCompleteMsg{}
		}

		var card contracts.TriageCard
		if err := json.Unmarshal(data, &card); err != nil {
			return pipelineErrorMsg{err: err}
		}
		return cardReceivedMsg{card: card}
	}
}

// Update handles messages and updates the model
func (m MainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case cardReceivedMsg:
		// New card arrived from broker - add to pending
		m.cardCount++
		item := Item{Card: msg.card, Rank: 0}
		m.pendingCards = append(m.pendingCards, item)

		// Track new jobs
		if !m.jobsDiscovered[msg.card.JobName] {
			m.jobsDiscovered[msg.card.JobName] = true
			m.header.AddJob(msg.card.JobName)
		}

		// Update header with pending count
		m.header.SetPendingCount(len(m.pendingCards))
		m.header.SetLoadStatus(m.status, m.cardCount, len(m.jobsDiscovered))

		// Keep listening for more cards
		if m.cardChan != nil {
			cmds = append(cmds, listenForCards(m.cardChan))
		}
		return m, tea.Batch(cmds...)

	case pipelineCompleteMsg:
		// Pipeline finished - auto-merge any pending cards
		m.status = StatusComplete
		m.header.SetLoadStatus(m.status, m.cardCount, len(m.jobsDiscovered))
		if len(m.pendingCards) > 0 {
			m.mergePendingCards()
		}
		return m, nil

	case pipelineErrorMsg:
		m.status = StatusError
		m.header.SetLoadStatus(m.status, m.cardCount, len(m.jobsDiscovered))
		return m, nil

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
		case "r":
			// Refresh: merge pending cards and re-rank
			if len(m.pendingCards) > 0 {
				m.mergePendingCards()
			}
			return m, nil
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

// mergePendingCards merges pending cards into the main list and re-ranks
func (m *MainModel) mergePendingCards() {
	// Add pending cards to hash map (grouping by hash)
	for _, item := range m.pendingCards {
		if existing, ok := m.hashMap[item.Card.MessageHash]; ok {
			// Increment recurrence count
			count := existing.GetRecurrence() + 1
			existing.Card.Metadata["recurrence_count"] = string(rune('0' + count))
		} else {
			itemCopy := item
			m.hashMap[item.Card.MessageHash] = &itemCopy
		}
	}

	// Clear pending
	m.pendingCards = nil
	m.header.SetPendingCount(0)

	// Rebuild sorted items list
	m.items = hashMapToSortedItems(m.hashMap)

	// Update list view
	m.listView.SetItems(m.items)
	m.applyFilter()

	// Update detail if needed
	if selectedItem, ok := m.listView.GetSelectedItem(); ok {
		m.updateDetailContent(selectedItem)
	}
}
