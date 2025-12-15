package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"destill-agent/src/broker"
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

// ConfidenceThreshold is the threshold for "high confidence" cards
// Cards below this are shown dimmed but still included
const ConfidenceThreshold = 0.80

// initialState holds the processed state from initial cards
type initialState struct {
	hashMap        map[string]*Item
	jobsDiscovered map[string]bool
	jobsFailed     map[string]bool
	items          []Item
	failedJobs     []JobInfo
	passedJobs     []JobInfo
	allJobs        []JobInfo
}

// brokerChannels holds the channels and context for broker subscriptions
type brokerChannels struct {
	cardChan     <-chan broker.Message
	progressChan <-chan broker.Message
	ctx          context.Context
	cancel       context.CancelFunc
}

// buildInitialState processes the initial cards and builds the state needed for the TUI.
func buildInitialState(cards []contracts.TriageCard) *initialState {
	hashMap := make(map[string]*Item)
	jobsDiscovered := make(map[string]bool)
	jobsFailed := make(map[string]bool)

	for _, card := range cards {
		if existing, ok := hashMap[card.MessageHash]; ok {
			// Increment recurrence count
			count := existing.GetRecurrence() + 1
			existing.Card.Metadata["recurrence_count"] = strconv.Itoa(count)
		} else {
			item := Item{Card: card, Rank: 0}
			hashMap[card.MessageHash] = &item
		}
		if !jobsDiscovered[card.JobName] {
			jobsDiscovered[card.JobName] = true
		}
		// Track if this job failed (exit_status != "0")
		if exitStatus, ok := card.Metadata["exit_status"]; ok && exitStatus != "0" {
			jobsFailed[card.JobName] = true
		}
	}

	// Build JobInfo lists sorted with failed jobs first
	var failedJobs []JobInfo
	var passedJobs []JobInfo
	for jobName := range jobsDiscovered {
		if jobsFailed[jobName] {
			failedJobs = append(failedJobs, JobInfo{Name: jobName, Failed: true})
		} else {
			passedJobs = append(passedJobs, JobInfo{Name: jobName, Failed: false})
		}
	}
	allJobs := append(failedJobs, passedJobs...)

	// Convert map to sorted slice
	items := hashMapToSortedItems(hashMap)

	return &initialState{
		hashMap:        hashMap,
		jobsDiscovered: jobsDiscovered,
		jobsFailed:     jobsFailed,
		items:          items,
		failedJobs:     failedJobs,
		passedJobs:     passedJobs,
		allJobs:        allJobs,
	}
}

// initializeHeader creates and configures the header with initial state.
func initializeHeader(styles *StyleConfig, state *initialState, status LoadStatus) Header {
	header := NewHeaderWithStyles("Destill Analysis", state.allJobs, styles)
	header.SetLoadStatus(status, len(state.items), len(state.jobsDiscovered))

	// Default to first failed job if any exist
	if len(state.failedJobs) > 0 {
		header.SetInitialFilter(state.failedJobs[0].Name, true)
	}

	return header
}

// initializeListView creates and configures the list view with initial items.
func initializeListView(state *initialState) View {
	listView := NewView()

	// Apply initial filter if defaulting to a failed job
	var initialItems []Item
	if len(state.failedJobs) > 0 {
		// Filter to show only the first failed job's items
		for _, item := range state.items {
			if item.Card.JobName == state.failedJobs[0].Name {
				initialItems = append(initialItems, item)
			}
		}
	} else {
		initialItems = state.items
	}

	listView.SetItems(initialItems)
	return listView
}

// subscribeToBroker sets up subscriptions to the broker channels.
// Returns nil channels if broker is nil.
func subscribeToBroker(brk broker.Broker) (*brokerChannels, error) {
	if brk == nil {
		return &brokerChannels{}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())

	cardChan, err := brk.Subscribe(ctx, contracts.TopicAnalysisFindings, "tui-consumer")
	if err != nil {
		cancel()
		return nil, err
	}

	progressChan, err := brk.Subscribe(ctx, contracts.TopicProgress, "tui-progress-consumer")
	if err != nil {
		cancel()
		return nil, err
	}

	return &brokerChannels{
		cardChan:     cardChan,
		progressChan: progressChan,
		ctx:          ctx,
		cancel:       cancel,
	}, nil
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
	broker         broker.Broker         // Message broker
	cardChan       <-chan broker.Message // Channel receiving cards from broker
	progressChan   <-chan broker.Message // Channel receiving progress updates
	pendingCards   []Item                // Cards waiting to be merged
	hashMap        map[string]*Item      // For grouping by hash
	status         LoadStatus            // Current loading status
	cardCount      int                   // Total cards received (above threshold)
	droppedCount   int                   // Cards dropped due to low confidence
	jobsDiscovered map[string]bool       // Jobs we've seen so far
	ctx            context.Context       // Context for broker operations
	cancel         context.CancelFunc    // Cancel function

	// Progress tracking
	progress ProgressModel // Progress model for showing loading state

	// Auto-switch tracking
	autoSwitchedToFailedJob bool // Track if we've auto-switched to first failed job
}

// Start initializes and runs the TUI with the provided triage cards.
func Start(cards []contracts.TriageCard) error {
	return StartWithBroker(nil, cards)
}

// StartWithBroker initializes the TUI in streaming mode with a message broker.
// If broker is nil, uses the provided initial cards only (no streaming).
// If broker is provided, subscribes to ci_failures_ranked for live updates.
// Invariant: If broker is not nil, initialCards must be empty.
func StartWithBroker(brk broker.Broker, initialCards []contracts.TriageCard) error {
	// Enforce invariant: broker and initialCards are mutually exclusive
	if brk != nil && len(initialCards) > 0 {
		return fmt.Errorf("invalid arguments: broker and initialCards are mutually exclusive (broker != nil requires empty initialCards)")
	}

	styles := DefaultStyles()
	state := buildInitialState(initialCards)

	// Determine initial status
	status := StatusComplete
	if brk != nil {
		status = StatusLoading
	}

	header := initializeHeader(styles, state, status)
	listView := initializeListView(state)

	channels, err := subscribeToBroker(brk)
	if err != nil {
		return err
	}

	model := MainModel{
		header:         header,
		listView:       listView,
		items:          state.items,
		styles:         styles,
		detailViewport: viewport.New(0, 0),
		ready:          false,
		broker:         brk,
		cardChan:       channels.cardChan,
		progressChan:   channels.progressChan,
		pendingCards:   nil,
		hashMap:        state.hashMap,
		status:         status,
		cardCount:      len(initialCards),
		droppedCount:   0,
		jobsDiscovered: state.jobsDiscovered,
		ctx:            channels.ctx,
		cancel:         channels.cancel,
		progress:       NewProgressModel(),
	}

	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err = p.Run()
	if channels.cancel != nil {
		channels.cancel()
	}
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
	var cmds []tea.Cmd
	if m.cardChan != nil {
		// Start listening for cards from broker
		cmds = append(cmds, listenForCards(m.cardChan))
	}
	if m.progressChan != nil {
		// Start listening for progress updates
		cmds = append(cmds, listenForProgress(m.progressChan))
	}
	return tea.Batch(cmds...)
}

// listenForCards returns a command that waits for the next card from the broker
func listenForCards(cardChan <-chan broker.Message) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-cardChan
		if !ok {
			// Channel closed, pipeline complete
			return pipelineCompleteMsg{}
		}

		var card contracts.TriageCard
		if err := json.Unmarshal(msg.Value, &card); err != nil {
			return pipelineErrorMsg{err: err}
		}
		return cardReceivedMsg{card: card}
	}
}

// listenForProgress returns a command that waits for the next progress update from the broker
func listenForProgress(progressChan <-chan broker.Message) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-progressChan
		if !ok {
			// Channel closed
			return nil
		}

		var update contracts.ProgressUpdate
		if err := json.Unmarshal(msg.Value, &update); err != nil {
			return nil
		}
		return ProgressMsg{
			Stage:   update.Stage,
			Current: update.Current,
			Total:   update.Total,
		}
	}
}

// Update handles messages and updates the model
func (m MainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case ProgressMsg:
		m.progress, cmd = m.progress.Update(msg)
		cmds = append(cmds, cmd)
		// Keep listening for more progress updates
		if m.progressChan != nil {
			cmds = append(cmds, listenForProgress(m.progressChan))
		}
		return m, tea.Batch(cmds...)

	case cardReceivedMsg:
		// New card arrived from broker - include all cards (low confidence shown dimmed)
		m.cardCount++

		// Track low confidence count for display
		if msg.card.ConfidenceScore < ConfidenceThreshold {
			m.droppedCount++ // Now means "low confidence" not "dropped"
			m.header.SetLowConfidenceCount(m.droppedCount)
		}

		// Track new jobs
		if !m.jobsDiscovered[msg.card.JobName] {
			m.jobsDiscovered[msg.card.JobName] = true
		}
		// Update job status in header (handles both new jobs and updating failed status)
		failed := false
		if exitStatus, ok := msg.card.Metadata["exit_status"]; ok && exitStatus != "0" {
			failed = true
		}
		m.header.AddJob(msg.card.JobName, failed)

		// Add card to pending
		item := Item{Card: msg.card, Rank: 0}
		m.pendingCards = append(m.pendingCards, item)

		// Auto-switch to first failed job when discovered
		if failed && !m.autoSwitchedToFailedJob && m.header.GetFilter() == "ALL" {
			m.autoSwitchedToFailedJob = true
			// Merge pending cards (including current one) so the failed job's findings are visible
			m.mergePendingCards()
			// Switch to this failed job
			m.header.CycleFilter() // This will cycle from ALL (index 0) to first job (index 1)
			m.applyFilter()
		}
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
