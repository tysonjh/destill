package tui

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// View manages the list of triage items.
type View struct {
	list     list.Model
	items    []Item
	delegate *Delegate
}

// NewView creates a new triage list view
func NewView() View {
	delegate := NewDelegate()
	l := list.New([]list.Item{}, &delegate, 0, 0)
	l.SetShowStatusBar(false)
	l.SetShowTitle(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)

	return View{
		list:     l,
		items:    []Item{},
		delegate: &delegate,
	}
}

// Update handles triage list updates
func (v View) Update(msg tea.Msg) (View, tea.Cmd) {
	var cmd tea.Cmd
	v.list, cmd = v.list.Update(msg)
	return v, cmd
}

// SetSize sets the list dimensions
func (v *View) SetSize(width, height int) {
	v.list.SetSize(width, height)
}

// SetItems sets the list items
func (v *View) SetItems(items []Item) {
	v.items = items

	// Calculate max rank and recurrence for column widths
	maxRank := 0
	maxRecurrence := 0
	for _, item := range items {
		if rank := item.Rank; rank > maxRank {
			maxRank = rank
		}
		if recur := item.GetRecurrence(); recur > maxRecurrence {
			maxRecurrence = recur
		}
	}

	// Update delegate column widths
	v.delegate.SetColumnWidths(maxRank, maxRecurrence)

	listItems := make([]list.Item, len(items))
	for i, item := range items {
		listItems[i] = item
	}
	v.list.SetItems(listItems)
}

// GetSelectedItem returns the currently selected triage item
func (v View) GetSelectedItem() (Item, bool) {
	if len(v.list.Items()) == 0 {
		return Item{}, false
	}
	return v.list.SelectedItem().(Item), true
}

// Render returns the string representation of the view
func (v View) Render() string {
	return v.list.View()
}

// GetDelegate returns the delegate for accessing column widths
func (v View) GetDelegate() *Delegate {
	return v.delegate
}
