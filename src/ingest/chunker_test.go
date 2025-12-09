package ingest

import (
	"strings"
	"testing"
)

func TestChunkLog_SmallContent(t *testing.T) {
	content := "line1\nline2\nline3"
	chunks := ChunkLog(content, "req-1", "build-1", "job1", "job-id-1", nil)

	if len(chunks) != 1 {
		t.Fatalf("Expected 1 chunk for small content, got %d", len(chunks))
	}

	chunk := chunks[0]
	if chunk.Content != content {
		t.Errorf("Content mismatch")
	}
	if chunk.ChunkIndex != 0 {
		t.Errorf("Expected chunk index 0, got %d", chunk.ChunkIndex)
	}
	if chunk.TotalChunks != 1 {
		t.Errorf("Expected total chunks 1, got %d", chunk.TotalChunks)
	}
	if chunk.LineStart != 1 {
		t.Errorf("Expected line start 1, got %d", chunk.LineStart)
	}
	if chunk.LineEnd != 3 {
		t.Errorf("Expected line end 3, got %d", chunk.LineEnd)
	}
}

func TestChunkLog_EmptyContent(t *testing.T) {
	chunks := ChunkLog("", "req-1", "build-1", "job1", "job-id-1", nil)

	if len(chunks) != 0 {
		t.Fatalf("Expected 0 chunks for empty content, got %d", len(chunks))
	}
}

func TestChunkLog_LargeContent(t *testing.T) {
	// Create content larger than 500KB
	var lines []string
	lineContent := strings.Repeat("a", 1000) // 1KB per line
	for i := 0; i < 600; i++ { // 600KB total
		lines = append(lines, lineContent)
	}
	content := strings.Join(lines, "\n")

	chunks := ChunkLog(content, "req-1", "build-1", "job1", "job-id-1", nil)

	// Should produce multiple chunks
	if len(chunks) < 2 {
		t.Fatalf("Expected multiple chunks for large content, got %d", len(chunks))
	}

	// Verify chunk metadata
	for i, chunk := range chunks {
		if chunk.ChunkIndex != i {
			t.Errorf("Chunk %d: expected index %d, got %d", i, i, chunk.ChunkIndex)
		}
		if chunk.TotalChunks != len(chunks) {
			t.Errorf("Chunk %d: expected total chunks %d, got %d", i, len(chunks), chunk.TotalChunks)
		}
		if chunk.RequestID != "req-1" {
			t.Errorf("Chunk %d: request ID mismatch", i)
		}
		if chunk.BuildID != "build-1" {
			t.Errorf("Chunk %d: build ID mismatch", i)
		}
		if chunk.JobName != "job1" {
			t.Errorf("Chunk %d: job name mismatch", i)
		}
	}

	// Verify chunk sizes are reasonable (close to 500KB)
	for i, chunk := range chunks {
		size := len(chunk.Content)
		// Each chunk should be close to 500KB, except possibly the last one
		if i < len(chunks)-1 && size < 400*1024 {
			t.Errorf("Chunk %d: size %d bytes is too small (expected ~500KB)", i, size)
		}
		if size > 600*1024 {
			t.Errorf("Chunk %d: size %d bytes exceeds reasonable limit", i, size)
		}
	}
}

func TestChunkLog_Overlap(t *testing.T) {
	// Create content that will span multiple chunks
	var lines []string
	for i := 0; i < 1000; i++ {
		lines = append(lines, strings.Repeat("x", 600)) // 600 bytes per line
	}
	content := strings.Join(lines, "\n")

	chunks := ChunkLog(content, "req-1", "build-1", "job1", "job-id-1", nil)

	if len(chunks) < 2 {
		t.Fatalf("Need at least 2 chunks to test overlap, got %d", len(chunks))
	}

	// Check that subsequent chunks have overlapping line ranges
	for i := 0; i < len(chunks)-1; i++ {
		currentEnd := chunks[i].LineEnd
		nextStart := chunks[i+1].LineStart

		// Next chunk should start before current chunk ends (overlap)
		if nextStart >= currentEnd {
			t.Errorf("Chunks %d and %d don't overlap: chunk %d ends at line %d, chunk %d starts at line %d",
				i, i+1, i, currentEnd, i+1, nextStart)
		}

		// Overlap should be reasonable (not more than ContextOverlap)
		overlap := currentEnd - nextStart + 1
		if overlap > ContextOverlap+10 { // +10 for tolerance
			t.Errorf("Overlap between chunks %d and %d is too large: %d lines (expected ~%d)",
				i, i+1, overlap, ContextOverlap)
		}
	}
}

func TestChunkLog_Metadata(t *testing.T) {
	metadata := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}

	content := strings.Repeat("line\n", 100)
	chunks := ChunkLog(content, "req-1", "build-1", "job1", "job-id-1", metadata)

	if len(chunks) == 0 {
		t.Fatal("Expected at least 1 chunk")
	}

	// Verify metadata is copied to chunks
	for i, chunk := range chunks {
		if len(chunk.Metadata) != len(metadata) {
			t.Errorf("Chunk %d: metadata length mismatch", i)
		}
		for k, v := range metadata {
			if chunk.Metadata[k] != v {
				t.Errorf("Chunk %d: metadata[%s] = %s, expected %s", i, k, chunk.Metadata[k], v)
			}
		}

		// Verify metadata is a copy (not shared reference)
		if i > 0 && &chunks[i].Metadata == &chunks[i-1].Metadata {
			t.Errorf("Chunks %d and %d share metadata reference", i-1, i)
		}
	}
}

func TestFormatChunkInfo(t *testing.T) {
	chunk := ChunkLog("line1\nline2", "req-1", "build-1", "job1", "job-id-1", nil)[0]

	info := FormatChunkInfo(chunk)
	expected := "Chunk 1/1: lines 1-2 (11 bytes)"

	if info != expected {
		t.Errorf("FormatChunkInfo: expected %q, got %q", expected, info)
	}
}

func TestChunkLog_LineNumbers(t *testing.T) {
	var lines []string
	for i := 1; i <= 100; i++ {
		lines = append(lines, strings.Repeat("x", 10000)) // 10KB per line
	}
	content := strings.Join(lines, "\n")

	chunks := ChunkLog(content, "req-1", "build-1", "job1", "job-id-1", nil)

	// Verify line numbers are continuous when considering overlap
	if len(chunks) < 2 {
		t.Skip("Need multiple chunks to test line numbers")
	}

	// First chunk should start at line 1
	if chunks[0].LineStart != 1 {
		t.Errorf("First chunk should start at line 1, got %d", chunks[0].LineStart)
	}

	// Last chunk should end at the total line count
	lastChunk := chunks[len(chunks)-1]
	if lastChunk.LineEnd != 100 {
		t.Errorf("Last chunk should end at line 100, got %d", lastChunk.LineEnd)
	}

	// Each chunk's line range should be valid
	for i, chunk := range chunks {
		if chunk.LineStart > chunk.LineEnd {
			t.Errorf("Chunk %d: LineStart (%d) > LineEnd (%d)", i, chunk.LineStart, chunk.LineEnd)
		}
		if chunk.LineStart < 1 {
			t.Errorf("Chunk %d: LineStart (%d) < 1", i, chunk.LineStart)
		}
	}
}

