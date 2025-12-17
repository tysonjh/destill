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

func TestCleanLogText_BuildkiteAPC(t *testing.T) {
	// Buildkite timestamp APC sequence: ESC _ bk;t=timestamp BEL
	input := "\x1b_bk;t=1732983165\x07%0|1732983165.701|fatal|rdkafka#producer-5|error message"
	expected := "%0|1732983165.701|fatal|rdkafka#producer-5|error message"

	result := CleanLogText(input)
	if result != expected {
		t.Errorf("Buildkite APC not stripped\nexpected: %q\ngot:      %q", expected, result)
	}
}

func TestCleanLogText_OSC(t *testing.T) {
	// OSC sequence for terminal title: ESC ] 0;title BEL
	input := "\x1b]0;Build Output\x07This is the actual content"
	expected := "This is the actual content"

	result := CleanLogText(input)
	if result != expected {
		t.Errorf("OSC not stripped\nexpected: %q\ngot:      %q", expected, result)
	}
}

func TestCleanLogText_DCS(t *testing.T) {
	// DCS sequence: ESC P ... ST (ESC \)
	input := "\x1bPsome device control\x1b\\actual content"
	expected := "actual content"

	result := CleanLogText(input)
	if result != expected {
		t.Errorf("DCS not stripped\nexpected: %q\ngot:      %q", expected, result)
	}
}

func TestCleanLogText_ANSI(t *testing.T) {
	// Standard ANSI color codes
	input := "\x1b[31mRed text\x1b[0m normal"
	expected := "Red text normal"

	result := CleanLogText(input)
	if result != expected {
		t.Errorf("ANSI codes not stripped\nexpected: %q\ngot:      %q", expected, result)
	}
}

func TestCleanLogText_LineEndings(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"CRLF", "line1\r\nline2", "line1\nline2"},
		{"CR", "line1\rline2", "line1\nline2"},
		{"Double CR LF", "line1\r\r\nline2", "line1\nline2"},
		{"Mixed", "a\r\nb\rc\r\r\nd", "a\nb\nc\nd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CleanLogText(tt.input)
			if result != tt.expected {
				t.Errorf("line endings not normalized\nexpected: %q\ngot:      %q", tt.expected, result)
			}
		})
	}
}

func TestCleanLogText_C0Controls(t *testing.T) {
	// C0 control characters (NUL, BEL standalone, etc.)
	input := "text\x00with\x07bell\x08and\x1fcontrols"
	expected := "textwithbellandcontrols"

	result := CleanLogText(input)
	if result != expected {
		t.Errorf("C0 controls not stripped\nexpected: %q\ngot:      %q", expected, result)
	}
}

func TestCleanLogText_PreservesTabsAndNewlines(t *testing.T) {
	input := "line1\n\tindented line2"
	expected := "line1\n\tindented line2"

	result := CleanLogText(input)
	if result != expected {
		t.Errorf("tabs/newlines incorrectly modified\nexpected: %q\ngot:      %q", expected, result)
	}
}

func TestCleanLogText_Complex(t *testing.T) {
	// Simulates real Buildkite log line
	input := "\x1b_bk;t=1732983165\x07\x1b[31m%0|1732983165.701|fatal|rdkafka#producer-5|\x1b[0m error\r\r\n"
	expected := "%0|1732983165.701|fatal|rdkafka#producer-5| error\n"

	result := CleanLogText(input)
	if result != expected {
		t.Errorf("complex log not cleaned properly\nexpected: %q\ngot:      %q", expected, result)
	}
}
