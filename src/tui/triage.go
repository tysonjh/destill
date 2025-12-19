package tui

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"destill-agent/src/broker"
	"destill-agent/src/contracts"
	"destill-agent/src/ranking"
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

// BrokerChannels holds the channels and context for broker subscriptions.
// Exported so callers can pre-subscribe before analysis starts.
type BrokerChannels struct {
	CardChan        <-chan broker.Message
	ProgressChan    <-chan broker.Message
	TestResultsChan <-chan broker.Message
	Ctx             context.Context
	Cancel          context.CancelFunc
}

// testResultMsg is sent when a test result arrives from the broker
type testResultMsg struct {
	result contracts.TestResult
}

// buildInitialState processes the initial cards and builds the state needed for the TUI.
func buildInitialState(cards []contracts.TriageCard) *initialState {
	hashMap := make(map[string]*Item)
	jobsDiscovered := make(map[string]bool)
	jobsFailed := make(map[string]bool)

	for _, card := range cards {
		if existing, ok := hashMap[card.MessageHash]; ok {
			// Increment recurrence count
			existing.Card.SetRecurrenceCount(existing.GetRecurrence() + 1)
		} else {
			item := Item{Card: card, Rank: 0}
			hashMap[card.MessageHash] = &item
		}
		if !jobsDiscovered[card.JobName] {
			jobsDiscovered[card.JobName] = true
		}
		// Track if this job failed
		if card.Metadata["job_state"] == "failed" {
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
	// Stay on "ALL" - failed job findings are already boosted to top by confidence
	return header
}

// initializeListView creates and configures the list view with initial items.
func initializeListView(state *initialState) View {
	listView := NewView()
	// Show all items - failed job findings are already boosted to top by confidence
	listView.SetItems(state.items)
	return listView
}

// SubscribeToBroker sets up subscriptions to the broker channels.
// Call this BEFORE submitting analysis to avoid race conditions with message delivery.
// Returns nil channels if broker is nil.
func SubscribeToBroker(brk broker.Broker) (*BrokerChannels, error) {
	if brk == nil {
		return &BrokerChannels{}, nil
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

	testResultsChan, err := brk.Subscribe(ctx, contracts.TopicTestResults, "tui-test-results-consumer")
	if err != nil {
		cancel()
		return nil, err
	}

	return &BrokerChannels{
		CardChan:        cardChan,
		ProgressChan:    progressChan,
		TestResultsChan: testResultsChan,
		Ctx:             ctx,
		Cancel:          cancel,
	}, nil
}

// TierFilter values for filtering by tier
const (
	TierFilterAll    = 0 // Show all tiers (default)
	TierFilterUnique = 1 // Unique failures only
	TierFilterNoise  = 2 // Noise only
)

// ViewMode represents the current view in the TUI
type ViewMode int

const (
	ViewLogs    ViewMode = iota // Log findings view (default, existing TUI)
	ViewSummary                 // Build summary overview
	ViewTests                   // Test failures view
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
	tierFilter     int // TierFilterAll (default), TierFilterUnique, or TierFilterNoise

	// Multi-view support
	viewMode     ViewMode
	summaryModel SummaryModel
	testsModel   TestsModel

	// Streaming support
	cardChan        <-chan broker.Message // Channel receiving cards from broker
	progressChan    <-chan broker.Message // Channel receiving progress updates
	testResultsChan <-chan broker.Message // Channel receiving test results
	pendingCards    []Item                // Cards waiting to be merged
	hashMap         map[string]*Item      // For grouping by hash
	status          LoadStatus            // Current loading status
	cardCount       int                   // Total cards received (above threshold)
	droppedCount    int                   // Cards dropped due to low confidence
	jobsDiscovered  map[string]bool       // Jobs we've seen so far
	ctx             context.Context       // Context for broker operations
	cancel          context.CancelFunc    // Cancel function

	// Test results tracking
	testResults []contracts.TestResult

	// Progress tracking
	progress ProgressModel // Progress model for showing loading state

	// Tier counts for header display
	uniqueCount int
	noiseCount  int

	// Warnings collected during analysis
	warnings []string
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
	// Subscribe to broker if provided
	channels, err := SubscribeToBroker(brk)
	if err != nil {
		return err
	}
	return StartWithChannels(channels, initialCards)
}

// StartWithChannels initializes the TUI with pre-subscribed broker channels.
// Use this when you need to subscribe BEFORE submitting analysis to avoid race conditions.
// If channels is nil or has nil CardChan, uses the provided initial cards only (no streaming).
// Invariant: If channels has non-nil CardChan, initialCards must be empty.
func StartWithChannels(channels *BrokerChannels, initialCards []contracts.TriageCard) error {
	// Determine if we're in streaming mode
	streaming := channels != nil && channels.CardChan != nil

	// Enforce invariant: streaming and initialCards are mutually exclusive
	if streaming && len(initialCards) > 0 {
		return fmt.Errorf("invalid arguments: streaming channels and initialCards are mutually exclusive")
	}

	styles := DefaultStyles()
	state := buildInitialState(initialCards)

	// Determine initial status
	status := StatusComplete
	if streaming {
		status = StatusLoading
	}

	header := initializeHeader(styles, state, status)
	listView := initializeListView(state)

	// Get tier counts for header
	unique, noise := getTierCounts(state.hashMap)

	// Extract channels (handle nil case)
	var cardChan <-chan broker.Message
	var progressChan <-chan broker.Message
	var testResultsChan <-chan broker.Message
	var ctx context.Context
	var cancel context.CancelFunc
	if channels != nil {
		cardChan = channels.CardChan
		progressChan = channels.ProgressChan
		testResultsChan = channels.TestResultsChan
		ctx = channels.Ctx
		cancel = channels.Cancel
	}

	model := MainModel{
		header:          header,
		listView:        listView,
		items:           state.items,
		styles:          styles,
		detailViewport:  viewport.New(0, 0),
		ready:           false,
		tierFilter:      TierFilterAll, // Show all by default
		viewMode:        ViewSummary,   // Start with summary view
		summaryModel:    NewSummaryModel(styles),
		testsModel:      NewTestsModel(styles),
		cardChan:        cardChan,
		progressChan:    progressChan,
		testResultsChan: testResultsChan,
		pendingCards:    nil,
		hashMap:         state.hashMap,
		status:          status,
		cardCount:       len(initialCards),
		droppedCount:    0,
		jobsDiscovered:  state.jobsDiscovered,
		ctx:             ctx,
		cancel:          cancel,
		progress:        NewProgressModel(),
		uniqueCount:     unique,
		noiseCount:      noise,
	}
	// Update header with tier counts and view mode
	model.header.SetTierCounts(unique, noise)
	model.header.SetViewMode(ViewSummary) // Start with summary view
	model.updateSummaryData()             // Initialize summary data
	// Apply default tier filter (hide noise)
	model.applyFilter()

	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err := p.Run()
	if cancel != nil {
		cancel()
	}
	return err
}

// hashMapToSortedItems converts the hash map to a sorted slice of items
// using the ranking package for tier-aware sorting.
func hashMapToSortedItems(hashMap map[string]*Item) []Item {
	// Extract cards for ranking
	cards := make([]contracts.TriageCard, 0, len(hashMap))
	for _, item := range hashMap {
		cards = append(cards, item.Card)
	}

	// Use ranking package to classify and sort by tier
	tiered := ranking.RankCards(cards)
	ranked := tiered.FlattenByTier()

	// Convert RankedCards to Items
	items := make([]Item, len(ranked))
	for i, rc := range ranked {
		items[i] = Item{
			Card: rc.Card,
			Rank: rc.Rank,
			Tier: rc.Tier,
		}
	}

	return items
}

// getTierCounts returns the count of unique failures and noise from a hash map
func getTierCounts(hashMap map[string]*Item) (unique, noise int) {
	cards := make([]contracts.TriageCard, 0, len(hashMap))
	for _, item := range hashMap {
		cards = append(cards, item.Card)
	}
	tiered := ranking.RankCards(cards)
	return tiered.Counts()
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
	if m.testResultsChan != nil {
		// Start listening for test results
		cmds = append(cmds, listenForTestResults(m.testResultsChan))
	}
	// Start spinner animation for loading screen
	cmds = append(cmds, SpinnerTick())
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
			Warning: update.Warning,
		}
	}
}

// listenForTestResults returns a command that waits for test results from the broker
func listenForTestResults(testResultsChan <-chan broker.Message) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-testResultsChan
		if !ok {
			// Channel closed
			return nil
		}

		var result contracts.TestResult
		if err := json.Unmarshal(msg.Value, &result); err != nil {
			return nil
		}
		return testResultMsg{result: result}
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
		// Track warnings
		if msg.Warning != "" {
			m.warnings = append(m.warnings, msg.Warning)
			m.summaryModel.SetWarnings(m.warnings)
		}
		// Keep listening for more progress updates
		if m.progressChan != nil {
			cmds = append(cmds, listenForProgress(m.progressChan))
		}
		return m, tea.Batch(cmds...)

	case testResultMsg:
		// New test result arrived from broker
		m.testResults = append(m.testResults, msg.result)
		// Update test summary
		m.updateTestSummary()
		// Keep listening for more test results
		if m.testResultsChan != nil {
			cmds = append(cmds, listenForTestResults(m.testResultsChan))
		}
		return m, tea.Batch(cmds...)

	case SpinnerTickMsg:
		m.progress, cmd = m.progress.Update(msg)
		return m, cmd

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
		failed := msg.card.Metadata["job_state"] == "failed"
		m.header.AddJob(msg.card.JobName, failed)

		// Add card to pending
		item := Item{Card: msg.card, Rank: 0}
		m.pendingCards = append(m.pendingCards, item)
		// Stay on "ALL" - failed job findings are boosted to top by confidence
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
		// Continue listening for test results and progress (warnings) - ArtifactAgent
		// may still be publishing after the analysis agent completes
		if m.testResultsChan != nil {
			cmds = append(cmds, listenForTestResults(m.testResultsChan))
		}
		if m.progressChan != nil {
			cmds = append(cmds, listenForProgress(m.progressChan))
		}
		return m, tea.Batch(cmds...)

	case pipelineErrorMsg:
		m.status = StatusError
		m.header.SetLoadStatus(m.status, m.cardCount, len(m.jobsDiscovered))
		// Continue listening for test results and progress even on error
		if m.testResultsChan != nil {
			cmds = append(cmds, listenForTestResults(m.testResultsChan))
		}
		if m.progressChan != nil {
			cmds = append(cmds, listenForProgress(m.progressChan))
		}
		return m, tea.Batch(cmds...)

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
			return m, tea.ClearScreen
		case "0":
			// Show all tiers
			m.tierFilter = TierFilterAll
			m.header.SetTierFilter(m.tierFilter)
			m.applyFilter()
			// Force full redraw to prevent terminal rendering glitches
			return m, tea.ClearScreen
		case "1":
			// Show unique failures only
			m.tierFilter = TierFilterUnique
			m.header.SetTierFilter(m.tierFilter)
			m.applyFilter()
			return m, tea.ClearScreen
		case "2":
			// Show noise only
			m.tierFilter = TierFilterNoise
			m.header.SetTierFilter(m.tierFilter)
			m.applyFilter()
			return m, tea.ClearScreen
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
			// Toggle focus to detail viewport (only in logs view)
			if m.viewMode == ViewLogs {
				m.detailFocused = !m.detailFocused
			}
			return m, nil
		case "s":
			// Switch to summary view
			m.viewMode = ViewSummary
			m.header.SetViewMode(ViewSummary)
			m.updateSummaryData()
			return m, tea.ClearScreen
		case "t":
			// Switch to tests view
			m.viewMode = ViewTests
			m.header.SetViewMode(ViewTests)
			return m, tea.ClearScreen
		case "l":
			// Switch to logs view
			m.viewMode = ViewLogs
			m.header.SetViewMode(ViewLogs)
			return m, tea.ClearScreen
		case "esc":
			// If detail is focused, return to list
			if m.detailFocused {
				m.detailFocused = false
				return m, nil
			}
			// Otherwise reset filter to ALL
			m.header.ResetFilter()
			m.tierFilter = TierFilterAll
			m.header.SetTierFilter(m.tierFilter)
			m.applyFilter()
			return m, nil
		}
	}

	// Route updates based on view mode and focus
	switch m.viewMode {
	case ViewTests:
		// Route to tests model for scrolling
		m.testsModel, cmd = m.testsModel.Update(msg)
		cmds = append(cmds, cmd)
	case ViewLogs:
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
	}

	return m, tea.Batch(cmds...)
}

// mergePendingCards merges pending cards into the main list and re-ranks
func (m *MainModel) mergePendingCards() {
	// Add pending cards to hash map (grouping by hash)
	for _, item := range m.pendingCards {
		if existing, ok := m.hashMap[item.Card.MessageHash]; ok {
			// Increment recurrence count
			existing.Card.SetRecurrenceCount(existing.GetRecurrence() + 1)
		} else {
			itemCopy := item
			m.hashMap[item.Card.MessageHash] = &itemCopy
		}
	}

	// Clear pending
	m.pendingCards = nil
	m.header.SetPendingCount(0)

	// Rebuild sorted items list with tier info
	m.items = hashMapToSortedItems(m.hashMap)

	// Update tier counts
	m.uniqueCount, m.noiseCount = getTierCounts(m.hashMap)
	m.header.SetTierCounts(m.uniqueCount, m.noiseCount)

	// Update list view
	m.listView.SetItems(m.items)
	m.applyFilter()

	// Update detail if needed
	if selectedItem, ok := m.listView.GetSelectedItem(); ok {
		m.updateDetailContent(selectedItem)
	}

	// Update summary data so it reflects latest state
	m.updateSummaryData()
}

// updateSummaryData updates the summary model with current data.
func (m *MainModel) updateSummaryData() {
	// Set size
	m.summaryModel.SetSize(m.width, m.height-4) // -4 for header

	// Set log findings counts
	m.summaryModel.SetLogFindings(m.uniqueCount, m.noiseCount, m.cardCount, m.droppedCount, len(m.header.availableJobs))

	// Count jobs by status
	failedJobs := 0
	passedJobs := 0
	otherJobs := 0
	for _, job := range m.header.availableJobs {
		if job.Failed {
			failedJobs++
		} else {
			passedJobs++
		}
	}
	m.summaryModel.SetJobCounts(failedJobs, passedJobs, otherJobs)

	// Build status
	status := "passed"
	if failedJobs > 0 {
		status = "failed"
	}
	m.summaryModel.SetBuildInfo(status, "", "")
}

// updateTestSummary builds a test summary from collected test results
func (m *MainModel) updateTestSummary() {
	if len(m.testResults) == 0 {
		return
	}

	// Count passed/failed
	passed := 0
	var failures []contracts.TestFailure
	for _, result := range m.testResults {
		if result.Passed {
			passed++
		} else {
			failures = append(failures, contracts.TestFailure{
				TestName:       result.TestName,
				FailureMessage: result.FailureMessage,
				FailureRate:    0, // TODO: integrate with flaky detection
				IsFlaky:        false,
			})
		}
	}

	summary := &contracts.TestSummary{
		TotalTests:    len(m.testResults),
		PassedCount:   passed,
		FailedCount:   len(failures),
		NovelFailures: failures, // For now, treat all failures as novel
	}

	m.summaryModel.SetTestSummary(summary)
	m.testsModel.SetTestData(summary, m.testResults)
}
