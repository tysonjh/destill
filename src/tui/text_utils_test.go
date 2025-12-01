package tui

import (
	"strings"
	"testing"
)

func TestWrap_ShortText(t *testing.T) {
	text := "hello world"
	width := 20
	result := Wrap(text, width)

	if result != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", result)
	}
}

func TestWrap_ExactWidth(t *testing.T) {
	text := "hello world"
	width := 11 // exactly the length
	result := Wrap(text, width)

	if result != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", result)
	}
}

func TestWrap_MultipleLines(t *testing.T) {
	text := "hello world this is a test"
	width := 15
	result := Wrap(text, width)

	lines := strings.Split(result, "\n")
	for i, line := range lines {
		lineWidth := VisualWidth(line)
		if lineWidth > width {
			t.Errorf("line %d exceeds width %d: width=%d, content='%s'", i, width, lineWidth, line)
		}
	}
}

func TestWrap_LongWord(t *testing.T) {
	// Simulate a long hash or URL
	text := "bk;t=[2025-11-30T15:32:45.123Z]%0|1732983165.701|fatal|rdkafka#consumer-1|"
	width := 40

	result := Wrap(text, width)
	lines := strings.Split(result, "\n")

	if len(lines) < 2 {
		t.Errorf("expected long word to be broken into multiple lines, got %d lines", len(lines))
	}

	for i, line := range lines {
		lineWidth := VisualWidth(line)
		if lineWidth > width {
			t.Errorf("line %d exceeds width %d: width=%d, content='%s'", i, width, lineWidth, line)
		}
	}

	// Verify all original content is preserved
	reconstructed := strings.ReplaceAll(result, "\n", "")
	if reconstructed != text {
		t.Errorf("content was modified during wrapping\nexpected: %s\ngot:      %s", text, reconstructed)
	}
}

func TestWrap_VeryLongWordShorterThanWidth(t *testing.T) {
	text := "verylongwordthatdoesntfit short"
	width := 20

	result := Wrap(text, width)
	lines := strings.Split(result, "\n")

	for i, line := range lines {
		lineWidth := VisualWidth(line)
		if lineWidth > width {
			t.Errorf("line %d exceeds width %d: width=%d, content='%s'", i, width, lineWidth, line)
		}
	}
}

func TestWrap_MultiByteCharacters(t *testing.T) {
	text := "Hello 世界 this is a test with emoji 🎉 and more text"
	width := 25

	result := Wrap(text, width)
	lines := strings.Split(result, "\n")

	for i, line := range lines {
		lineWidth := VisualWidth(line)
		if lineWidth > width {
			t.Errorf("line %d exceeds width %d: width=%d, content='%s'", i, width, lineWidth, line)
		}
	}
}

func TestWrap_EmptyString(t *testing.T) {
	result := Wrap("", 20)
	if result != "" {
		t.Errorf("expected empty string, got '%s'", result)
	}
}

func TestWrap_ZeroWidth(t *testing.T) {
	text := "hello world"
	result := Wrap(text, 0)
	if result != text {
		t.Errorf("expected original text for zero width, got '%s'", result)
	}
}

func TestTruncate_WithEllipsis(t *testing.T) {
	text := "this is a very long text"
	maxLen := 10
	result := Truncate(text, maxLen, true)

	width := VisualWidth(result)
	if width > maxLen {
		t.Errorf("truncated text exceeds maxLen %d: width=%d, content='%s'", maxLen, width, result)
	}

	if !strings.HasSuffix(result, "...") {
		t.Errorf("expected ellipsis, got '%s'", result)
	}
}

func TestTruncate_WithoutEllipsis(t *testing.T) {
	text := "this is a very long text"
	maxLen := 10
	result := Truncate(text, maxLen, false)

	width := VisualWidth(result)
	if width > maxLen {
		t.Errorf("truncated text exceeds maxLen %d: width=%d, content='%s'", maxLen, width, result)
	}

	if strings.HasSuffix(result, "...") {
		t.Errorf("unexpected ellipsis, got '%s'", result)
	}
}

func TestTruncateAndPad(t *testing.T) {
	text := "short"
	width := 10
	result := TruncateAndPad(text, width, false)

	resultWidth := VisualWidth(result)
	if resultWidth != width {
		t.Errorf("expected width %d, got %d for '%s'", width, resultWidth, result)
	}
}
