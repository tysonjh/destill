package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestProgressModel_InitialState(t *testing.T) {
	model := NewProgressModel()

	if model.done {
		t.Error("expected not done initially")
	}

	view := model.View()
	if !strings.Contains(view, "Downloading build data") {
		t.Errorf("expected view to contain 'Downloading build data', got: %s", view)
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
