package contracts

// Finding history constants
const (
	// FindingWindowSize is the number of recent builds to consider for finding history.
	// Larger than FlakeWindowSize since findings are less frequent than test runs.
	FindingWindowSize = 50

	// FindingMinSamples is the minimum builds with this finding to establish a pattern.
	FindingMinSamples = 3
)
