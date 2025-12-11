// Package ingest provides log ingestion and chunking functionality.
package ingest

import (
	"bufio"
	"fmt"
	"strings"

	"destill-agent/src/contracts"
)

const (
	// TargetChunkSize is the target size for each chunk (500KB)
	TargetChunkSize = 500 * 1024

	// ContextOverlap is the number of lines to overlap between chunks
	// This helps preserve context at chunk boundaries
	ContextOverlap = 50
)

// ChunkLog splits a log into ~500KB chunks with line overlap.
// Each chunk maintains context by overlapping 50 lines with the previous chunk.
func ChunkLog(content string, requestID, buildID, jobName, jobID string, metadata map[string]string) []contracts.LogChunk {
	if len(content) == 0 {
		return []contracts.LogChunk{}
	}

	// Split into lines for processing
	scanner := bufio.NewScanner(strings.NewReader(content))
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if len(lines) == 0 {
		return []contracts.LogChunk{}
	}

	// If content is small, return single chunk
	if len(content) <= TargetChunkSize {
		chunk := contracts.LogChunk{
			RequestID:   requestID,
			BuildID:     buildID,
			JobName:     jobName,
			JobID:       jobID,
			ChunkIndex:  0,
			TotalChunks: 1,
			Content:     content,
			LineStart:   1,
			LineEnd:     len(lines),
			Metadata:    metadata,
		}
		return []contracts.LogChunk{chunk}
	}

	// Build chunks with target size
	var chunks []contracts.LogChunk
	currentLines := []string{}
	currentSize := 0
	lineStart := 1
	overlapBuffer := []string{} // Keep last N lines for overlap

	for i, line := range lines {
		lineSize := len(line) + 1 // +1 for newline

		// Check if adding this line would exceed target size
		if currentSize+lineSize > TargetChunkSize && len(currentLines) > 0 {
			// Create chunk from current lines
			chunkContent := strings.Join(currentLines, "\n")
			chunk := contracts.LogChunk{
				RequestID:   requestID,
				BuildID:     buildID,
				JobName:     jobName,
				JobID:       jobID,
				ChunkIndex:  len(chunks),
				TotalChunks: 0, // Will be set after all chunks are created
				Content:     chunkContent,
				LineStart:   lineStart,
				LineEnd:     i,
				Metadata:    copyMetadata(metadata),
			}
			chunks = append(chunks, chunk)

			// Prepare for next chunk with overlap
			// Keep last ContextOverlap lines as overlap buffer
			if len(currentLines) >= ContextOverlap {
				overlapBuffer = currentLines[len(currentLines)-ContextOverlap:]
			} else {
				overlapBuffer = currentLines
			}

			// Start new chunk with overlap
			lineStart = i + 1 - len(overlapBuffer)
			currentLines = make([]string, len(overlapBuffer))
			copy(currentLines, overlapBuffer)
			currentSize = 0
			for _, ol := range overlapBuffer {
				currentSize += len(ol) + 1
			}
		}

		// Add current line
		currentLines = append(currentLines, line)
		currentSize += lineSize
	}

	// Add final chunk if there are remaining lines
	if len(currentLines) > 0 {
		chunkContent := strings.Join(currentLines, "\n")
		chunk := contracts.LogChunk{
			RequestID:   requestID,
			BuildID:     buildID,
			JobName:     jobName,
			JobID:       jobID,
			ChunkIndex:  len(chunks),
			TotalChunks: 0, // Will be set below
			Content:     chunkContent,
			LineStart:   lineStart,
			LineEnd:     len(lines),
			Metadata:    copyMetadata(metadata),
		}
		chunks = append(chunks, chunk)
	}

	// Update total chunks count
	totalChunks := len(chunks)
	for i := range chunks {
		chunks[i].TotalChunks = totalChunks
	}

	return chunks
}

// copyMetadata creates a copy of the metadata map.
func copyMetadata(original map[string]string) map[string]string {
	if original == nil {
		return make(map[string]string)
	}
	copy := make(map[string]string, len(original))
	for k, v := range original {
		copy[k] = v
	}
	return copy
}

// FormatChunkInfo returns a human-readable summary of chunk information.
func FormatChunkInfo(chunk contracts.LogChunk) string {
	return fmt.Sprintf("Chunk %d/%d: lines %d-%d (%d bytes)",
		chunk.ChunkIndex+1,
		chunk.TotalChunks,
		chunk.LineStart,
		chunk.LineEnd,
		len(chunk.Content))
}
