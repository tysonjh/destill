package tui

import (
	"strings"
)

// itemMatchesQuery checks if an item matches the search query.
// Searches in message, job name, hash, severity, and context lines.
func itemMatchesQuery(item Item, query string) bool {
	// Search in primary fields
	if strings.Contains(strings.ToLower(item.Card.NormalizedMsg), query) ||
		strings.Contains(strings.ToLower(item.Card.JobName), query) ||
		strings.Contains(strings.ToLower(item.Card.MessageHash), query) ||
		strings.Contains(strings.ToLower(item.Card.Severity), query) {
		return true
	}

	// Search in PreContext
	for _, line := range item.GetPreContext() {
		if strings.Contains(strings.ToLower(line), query) {
			return true
		}
	}

	// Search in PostContext
	for _, line := range item.GetPostContext() {
		if strings.Contains(strings.ToLower(line), query) {
			return true
		}
	}

	return false
}

// applyFilter filters items based on job filter and search query
func (m *MainModel) applyFilter() {
	filter := m.header.GetFilter()

	// 1. Filter by Job
	var filtered []Item
	if filter == "ALL" {
		filtered = m.items
	} else {
		for _, item := range m.items {
			if item.Card.JobName == filter {
				filtered = append(filtered, item)
			}
		}
	}

	// 2. Filter by Search Query
	if m.searchQuery != "" {
		query := strings.ToLower(m.searchQuery)
		var searchFiltered []Item
		for _, item := range filtered {
			if itemMatchesQuery(item, query) {
				searchFiltered = append(searchFiltered, item)
			}
		}
		filtered = searchFiltered
	}

	m.listView.SetItems(filtered)
	// Update detail content for new selection
	if selectedItem, ok := m.listView.GetSelectedItem(); ok {
		m.updateDetailContent(selectedItem)
	}
}
