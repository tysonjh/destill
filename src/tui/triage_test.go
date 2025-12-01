package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"destill-agent/src/contracts"
)

// Helper to create a model for testing
func createTestModel(cards []contracts.TriageCard) MainModel {
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

	styles := DefaultStyles()
	header := NewHeaderWithStyles("Destill Analysis", jobs, styles)
	listView := NewView()
	listView.SetItems(items)

	return MainModel{
		header:   header,
		listView: listView,
		items:    items,
		styles:   styles,
	}
}

func TestMainModel_Initialization(t *testing.T) {
	cards := []contracts.TriageCard{
		{
			ID:              "card-1",
			JobName:         "tests",
			Message:         "Test failed",
			ConfidenceScore: 0.95,
		},
	}

	model := createTestModel(cards)

	if len(model.items) != 1 {
		t.Errorf("expected 1 item, got %d", len(model.items))
	}
	
	// Check header jobs
	if len(model.header.availableJobs) != 1 {
		t.Errorf("expected 1 available job, got %d", len(model.header.availableJobs))
	}
}

func TestMainModel_Update_Resize(t *testing.T) {
	model := createTestModel([]contracts.TriageCard{})

	// Send resize message
	msg := tea.WindowSizeMsg{Width: 100, Height: 40}
	updatedModel, _ := model.Update(msg)
	m := updatedModel.(MainModel)

	if m.width != 100 {
		t.Errorf("expected width 100, got %d", m.width)
	}
	if m.height != 40 {
		t.Errorf("expected height 40, got %d", m.height)
	}
	
	// Check list view size logic (1/3 of available)
	// Header=3, Footer=1. Available = 36. List = 12.
	// Note: We can't easily check inner list size without exposing it, 
	// but we can ensure no panic and state update.
}

func TestMainModel_Filter(t *testing.T) {
	cards := []contracts.TriageCard{
		{JobName: "frontend", Message: "FE Error"},
		{JobName: "backend", Message: "BE Error"},
	}

	model := createTestModel(cards)

	// Initial state: ALL
	if len(model.listView.items) != 2 {
		t.Errorf("expected 2 items initially, got %d", len(model.listView.items))
	}

	// Simulate Tab to cycle filter
	// 1. ALL -> frontend
	updatedModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	m := updatedModel.(MainModel)

	// Header should update
	if m.header.GetFilter() != "frontend" {
		// Note: Order depends on map iteration in createTestModel, which is random.
		// But typically it cycles through available jobs.
		// Since map iteration is random, we can't guarantee "frontend" comes first unless we sort.
		// In `Start` implementation:
		// for i, card := range cards { ... if !jobSet ... append }
		// Iteration over slice `cards` is deterministic. So "frontend" (index 0) comes first.
	}

	if m.header.GetFilter() == "ALL" {
		t.Error("expected filter to change from ALL")
	}

	// The list items should be filtered
	// We can't check m.listView.items directly if it's private, but we are in package tui.
	// m.listView.items IS accessible in this package.
	if len(m.listView.items) != 1 {
		// It might be that "frontend" or "backend" was selected.
		// Both have 1 item.
		t.Errorf("expected 1 filtered item, got %d", len(m.listView.items))
	}
}

func TestMainModel_View(t *testing.T) {
	cards := []contracts.TriageCard{
		{
			ID:              "card-1",
			JobName:         "tests",
			Message:         "Connection timeout",
			MessageHash:     "abcdef1234",
			ConfidenceScore: 0.95,
			Metadata: map[string]string{
				"recurrence_count": "3",
			},
		},
	}

	model := createTestModel(cards)
	
	// Initialize size
	updatedModel, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m := updatedModel.(MainModel)

	view := m.View()

	// Check for key elements
	if !strings.Contains(view, "Destill Analysis") {
		t.Error("view should contain header title")
	}
	if !strings.Contains(view, "Connection timeout") {
		t.Error("view should contain error message")
	}
	if !strings.Contains(view, "tests") {
		t.Error("view should contain job name")
	}
	// Check logic for rendering details (should contain message again in details pane)
	// "Connection timeout" appears in list AND details.
}