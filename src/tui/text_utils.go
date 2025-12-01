package tui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

var (
	// APC sequences: ESC _ <content> BEL or ESC _ <content> ST
	// Used by Buildkite for timestamps: \x1b_bk;t=1234567890\x07
	apcPattern = regexp.MustCompile("\x1b_[^\x07\x1b]*[\x07]|\x1b_[^\x1b]*\x1b\\\\")

	// OSC sequences: ESC ] <content> BEL or ESC ] <content> ST
	oscPattern = regexp.MustCompile("\x1b\\][^\x07\x1b]*[\x07]|\x1b\\][^\x1b]*\x1b\\\\")
)

// CleanLogText removes Buildkite escape sequences and normalizes line endings
// This should be called on raw log content before processing
func CleanLogText(s string) string {
	// Remove APC sequences (Buildkite timestamps)
	s = apcPattern.ReplaceAllString(s, "")

	// Remove OSC sequences
	s = oscPattern.ReplaceAllString(s, "")

	// Strip standard ANSI escape codes
	s = ansi.Strip(s)

	// Normalize line endings: \r\r\n -> \n, \r\n -> \n, \r -> \n
	s = strings.ReplaceAll(s, "\r\r\n", "\n")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	return s
}

// VisualWidth returns the display width of text, accounting for multi-byte characters
// and stripping ANSI escape codes for accurate calculation.
func VisualWidth(s string) int {
	return lipgloss.Width(s)
}

// StripAnsi removes ANSI escape codes from the string
func StripAnsi(s string) string {
	return ansi.Strip(s)
}

// Truncate truncates text to maxLen characters (visual width) with optional ellipsis.
// This function strips ANSI codes to ensure strict length compliance and avoid
// broken escape sequences in tabular layouts.
func Truncate(s string, maxLen int, ellipsis bool) string {
	// Replace newlines and tabs with spaces to ensure single-line display
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.TrimSpace(s)

	// Strip ANSI to ensure we don't truncate in the middle of an escape sequence
	// and to guarantee the resulting visual width matches expectation.
	s = StripAnsi(s)

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
			// Break the long word
			for VisualWidth(word) > width {
				chunk := runewidth.Truncate(word, width, "")
				result.WriteString(chunk)
				result.WriteString("\n")
				word = word[len(chunk):]
			}
			result.WriteString(word)
			lineLength = VisualWidth(word)
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
