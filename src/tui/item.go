package tui

import "destill-agent/src/contracts"

// Item represents an item that can be displayed in the triage list.
// It wraps the domain TriageCard and implements bubbles/list.Item.
type Item struct {
	Card contracts.TriageCard
	Rank int
}

// FilterValue is the value used for fuzzy filtering.
func (i Item) FilterValue() string { return i.Card.NormalizedMsg }

// Title returns the primary text for the item (required by list.Item).
func (i Item) Title() string { return i.Card.NormalizedMsg }

// Description returns the secondary text for the item (required by list.Item).
func (i Item) Description() string { return i.Card.JobName }

// Helper methods for easier data access in the view

// GetRecurrence returns the recurrence count for this item.
func (i Item) GetRecurrence() int {
	return i.Card.GetRecurrenceCount()
}

func (i Item) GetPreContext() []string {
	return i.Card.PreContext
}

func (i Item) GetPostContext() []string {
	return i.Card.PostContext
}
