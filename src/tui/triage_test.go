package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"destill-agent/src/contracts"
)

func Test_newTriageModel(t *testing.T) {
	cards := []contracts.TriageCard{
		{
			ID:              "card-1",
			Source:          "buildkite",
			JobName:         "tests",
			Severity:        "ERROR",
			Message:         "Test failed",
			MessageHash:     "abcd1234",
			ConfidenceScore: 0.95,
			Metadata: map[string]string{
				"recurrence_count": "5",
			},
		},
	}

	model := newTriageModel(cards)

	if len(model.cards) != 1 {
		t.Errorf("expected 1 card, got %d", len(model.cards))
	}

	if model.cursor != 0 {
		t.Errorf("expected cursor at 0, got %d", model.cursor)
	}
}

func TestTriageModel_EmptyCards(t *testing.T) {
	model := newTriageModel([]contracts.TriageCard{})

	// Set window size to ensure View doesn't return "Initializing..."
	updatedModel, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model = updatedModel.(TriageModel)

	view := model.View()
	if view == "" {
		t.Error("expected non-empty view for empty cards")
	}

	expected := "No failures detected or analyzed.\n"
	if view != expected {
		t.Errorf("expected empty message:\n%q\ngot:\n%q", expected, view)
	}
}

func TestTriageModel_Navigation(t *testing.T) {
	cards := []contracts.TriageCard{
		{MessageHash: "hash1", Message: "Error 1", ConfidenceScore: 0.9},
		{MessageHash: "hash2", Message: "Error 2", ConfidenceScore: 0.8},
		{MessageHash: "hash3", Message: "Error 3", ConfidenceScore: 0.7},
	}

	model := newTriageModel(cards)

	// Test down navigation
	updatedModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updatedModel.(TriageModel)
	if model.cursor != 1 {
		t.Errorf("expected cursor at 1 after down, got %d", model.cursor)
	}

	// Test j key navigation (vim-style down)
	updatedModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model = updatedModel.(TriageModel)
	if model.cursor != 2 {
		t.Errorf("expected cursor at 2 after j, got %d", model.cursor)
	}

	// Test boundary - can't go past end
	updatedModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updatedModel.(TriageModel)
	if model.cursor != 2 {
		t.Errorf("expected cursor to stay at 2 at boundary, got %d", model.cursor)
	}

	// Test up navigation
	updatedModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	model = updatedModel.(TriageModel)
	if model.cursor != 1 {
		t.Errorf("expected cursor at 1 after up, got %d", model.cursor)
	}

	// Test k key navigation (vim-style up)
	updatedModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	model = updatedModel.(TriageModel)
	if model.cursor != 0 {
		t.Errorf("expected cursor at 0 after k, got %d", model.cursor)
	}

	// Test boundary - can't go before start
	updatedModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	model = updatedModel.(TriageModel)
	if model.cursor != 0 {
		t.Errorf("expected cursor to stay at 0 at boundary, got %d", model.cursor)
	}
}

func TestTriageModel_View(t *testing.T) {
	cards := []contracts.TriageCard{
		{
			ID:              "card-1",
			Source:          "buildkite",
			JobName:         "tests",
			Severity:        "ERROR",
			Message:         "Connection timeout after 30s",
			MessageHash:     "abcdef1234567890",
			ConfidenceScore: 0.95,
			Metadata: map[string]string{
				"recurrence_count": "3",
			},
		},
	}

	model := newTriageModel(cards)

	// Set window size to ensure View doesn't return "Initializing..."
	updatedModel, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model = updatedModel.(TriageModel)

	view := model.View()

	// Check that view contains expected elements
	if !contains(view, "Destill") {
		t.Error("view should contain title")
	}

	if !contains(view, "Confidence") {
		t.Error("view should contain Confidence header")
	}

	if !contains(view, "Recurrence") {
		t.Error("view should contain Recurrence header")
	}

	if !contains(view, "Hash") {
		t.Error("view should contain Hash header")
	}

	if !contains(view, "Error Message") {
		t.Error("view should contain Error Message header")
	}

	if !contains(view, "0.95") {
		t.Error("view should contain confidence score")
	}

	if !contains(view, "3") {
		t.Error("view should contain recurrence count")
	}

	if !contains(view, "abcdef12") {
		t.Error("view should contain first 8 chars of hash")
	}

	if !contains(view, "Connection timeout") {
		t.Error("view should contain log message")
	}

	if !contains(view, "quit") {
		t.Error("view should contain help text")
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
