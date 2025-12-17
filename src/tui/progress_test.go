package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestProgressModel_InitialState(t *testing.T) {
	model := NewProgressModel()

	if model.stage != "" {
		t.Errorf("expected empty stage, got %s", model.stage)
	}
	if model.done {
		t.Error("expected not done initially")
	}
}

func TestProgressModel_UpdateWithStage(t *testing.T) {
	model := NewProgressModel()

	msg := ProgressMsg{
		Stage:   "Downloading build metadata",
		Current: 0,
		Total:   0,
	}

	model, _ = model.Update(msg)

	if model.stage != "Downloading build metadata" {
		t.Errorf("expected stage 'Downloading build metadata', got %s", model.stage)
	}

	view := model.View()
	if !strings.Contains(view, "Downloading build metadata") {
		t.Errorf("expected view to contain stage, got: %s", view)
	}
}

func TestProgressModel_UpdateWithProgress(t *testing.T) {
	model := NewProgressModel()

	msg := ProgressMsg{
		Stage:   "Fetching logs",
		Current: 3,
		Total:   5,
	}

	model, _ = model.Update(msg)

	view := model.View()
	if !strings.Contains(view, "3/5") {
		t.Errorf("expected view to contain '3/5', got: %s", view)
	}
	if !strings.Contains(view, "60%") {
		t.Errorf("expected view to contain '60%%', got: %s", view)
	}
}

func TestProgressModel_Complete(t *testing.T) {
	model := NewProgressModel()

	msg := ProgressMsg{
		Stage:   "complete",
		Current: 0,
		Total:   0,
	}

	model, _ = model.Update(msg)

	if !model.done {
		t.Error("expected model to be done after 'complete' stage")
	}

	view := model.View()
	if !strings.Contains(view, "Complete") {
		t.Errorf("expected view to contain 'Complete', got: %s", view)
	}
}

func TestProgressModel_ImplementsUpdate(t *testing.T) {
	var _ interface {
		Update(tea.Msg) (ProgressModel, tea.Cmd)
	} = ProgressModel{}
}
