package tui

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

// VisualWidth returns the display width of text, accounting for multi-byte characters
func VisualWidth(s string) int {
	return runewidth.StringWidth(s)
}

// Truncate truncates text to maxLen characters (visual width) with optional ellipsis
func Truncate(s string, maxLen int, ellipsis bool) string {
	s = strings.TrimSpace(s)
	if maxLen <= 0 {
		return ""
	}

	visualWidth := VisualWidth(s)
	if visualWidth > maxLen {
		if ellipsis && maxLen > 3 {
			// Truncate to fit maxLen-3 visual characters, then add ellipsis
			return runewidth.Truncate(s, maxLen-3, "") + "..."
		}
		return runewidth.Truncate(s, maxLen, "")
	}
	return s
}

// TruncateAndPad truncates text with optional ellipsis and pads to exact width
// Used for table cells to maintain consistent column widths
func TruncateAndPad(s string, width int, ellipsis bool) string {
	s = Truncate(s, width, ellipsis)
	visualWidth := VisualWidth(s)
	if visualWidth < width {
		return s + strings.Repeat(" ", width-visualWidth)
	}
	return s
}

// Wrap wraps text to the specified width, breaking on word boundaries when possible
// Long words that exceed width are broken mid-word
func Wrap(text string, width int) string {
	if width <= 0 {
		return text
	}

	var result strings.Builder
	words := strings.Fields(text)
	if len(words) == 0 {
		return text
	}

	lineLength := 0
	for _, word := range words {
		wordLen := VisualWidth(word)

		// If word is longer than width, break it mid-word
		if wordLen > width {
			// If there's content on current line, break to new line first
			if lineLength > 0 {
				result.WriteString("\n")
				lineLength = 0
			}

			// Break the long word into chunks
			for len(word) > 0 {
				// Truncate to fit width and add to result
				chunk := runewidth.Truncate(word, width, "")
				chunkLen := VisualWidth(chunk)
				result.WriteString(chunk)

				// Remove the chunk from word using runewidth-aware slicing
				// Count runes until we reach the visual width
				runeCount := 0
				currentWidth := 0
				for _, r := range word {
					if currentWidth >= chunkLen {
						break
					}
					runeCount++
					currentWidth += runewidth.RuneWidth(r)
				}
				// Slice by rune count
				runes := []rune(word)
				if runeCount < len(runes) {
					word = string(runes[runeCount:])
				} else {
					word = ""
				}

				// Add newline if there's more to process
				if len(word) > 0 {
					result.WriteString("\n")
				}
			}
			lineLength = 0
			continue
		}

		// Normal word handling
		if lineLength == 0 {
			// First word on line
			result.WriteString(word)
			lineLength = wordLen
		} else if lineLength+1+wordLen <= width {
			// Word fits on current line
			result.WriteString(" ")
			result.WriteString(word)
			lineLength += 1 + wordLen
		} else {
			// Word doesn't fit, start new line
			result.WriteString("\n")
			result.WriteString(word)
			lineLength = wordLen
		}
	}

	return result.String()
}

// SplitLines splits text by newlines, returning empty slice if text is empty
func SplitLines(text string) []string {
	if text == "" {
		return []string{}
	}
	return strings.Split(text, "\n")
}
