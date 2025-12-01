package tui

import (
	"strings"
)

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
		var searchFiltered []Item
		query := strings.ToLower(m.searchQuery)
		for _, item := range filtered {
			// Search in message, job name, hash, severity
			if strings.Contains(strings.ToLower(item.Card.Message), query) ||
				strings.Contains(strings.ToLower(item.Card.JobName), query) ||
				strings.Contains(strings.ToLower(item.Card.MessageHash), query) ||
				strings.Contains(strings.ToLower(item.Card.Severity), query) {
				searchFiltered = append(searchFiltered, item)
				continue
			}
			// Search in PreContext
			for _, line := range item.GetPreContext() {
				if strings.Contains(strings.ToLower(line), query) {
					searchFiltered = append(searchFiltered, item)
					goto nextItem
				}
			}
			// Search in PostContext
			for _, line := range item.GetPostContext() {
				if strings.Contains(strings.ToLower(line), query) {
					searchFiltered = append(searchFiltered, item)
					goto nextItem
				}
			}
		nextItem:
		}
		filtered = searchFiltered
	}

	m.listView.SetItems(filtered)
	// Update detail content for new selection
	if selectedItem, ok := m.listView.GetSelectedItem(); ok {
		m.updateDetailContent(selectedItem)
	}
}
