// Package blame provides blame correlation functionality to identify
// which code changes likely caused build failures.
package blame

import (
	"regexp"
	"sort"
	"strings"

	"destill-agent/src/contracts"
	"destill-agent/src/githubactions"
)

// Candidate represents a file/function that may have caused the failure.
type Candidate struct {
	File     string   `json:"file"`
	Function string   `json:"function,omitempty"`
	Score    float64  `json:"score"`
	Evidence []string `json:"evidence"`
}

// Result contains the blame correlation analysis.
type Result struct {
	Commit     string      `json:"commit"`
	Message    string      `json:"message,omitempty"`
	Candidates []Candidate `json:"candidates"`
	DiffURL    string      `json:"diff_url,omitempty"`
}

// Correlate matches changed files from a commit against file references in findings.
// Returns candidates sorted by score (highest first).
func Correlate(commit *githubactions.Commit, cards []contracts.TriageCard, owner, repo string) *Result {
	if commit == nil || len(commit.Files) == 0 {
		return &Result{Commit: "", Candidates: nil}
	}

	// Extract file references from all findings
	refs := extractFileRefs(cards)

	// Build a map of changed files for quick lookup
	changedFiles := make(map[string]*githubactions.CommitFile)
	for i := range commit.Files {
		f := &commit.Files[i]
		changedFiles[f.Filename] = f
		// Also index by basename for partial matches
		parts := strings.Split(f.Filename, "/")
		if len(parts) > 1 {
			changedFiles[parts[len(parts)-1]] = f
		}
	}

	// Score each changed file
	candidateMap := make(map[string]*Candidate)

	for _, cf := range commit.Files {
		candidate := &Candidate{
			File:     cf.Filename,
			Score:    0,
			Evidence: []string{},
		}

		for _, ref := range refs {
			score, evidence := matchFileRef(cf, ref)
			if score > 0 {
				candidate.Score += score
				candidate.Evidence = append(candidate.Evidence, evidence)
			}
		}

		if candidate.Score > 0 {
			candidateMap[cf.Filename] = candidate
		}
	}

	// Convert to slice and sort by score
	candidates := make([]Candidate, 0, len(candidateMap))
	for _, c := range candidateMap {
		candidates = append(candidates, *c)
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	// Limit to top 5 candidates
	if len(candidates) > 5 {
		candidates = candidates[:5]
	}

	// Truncate commit message to first line
	message := commit.Message
	if idx := strings.Index(message, "\n"); idx > 0 {
		message = message[:idx]
	}

	return &Result{
		Commit:     commit.SHA,
		Message:    message,
		Candidates: candidates,
		DiffURL:    "https://github.com/" + owner + "/" + repo + "/commit/" + commit.SHA,
	}
}

// FileRef represents a file reference extracted from a finding.
type FileRef struct {
	Path     string
	Line     int
	Function string
	Source   string // Which finding/context it came from
}

// extractFileRefs extracts file path references from findings.
func extractFileRefs(cards []contracts.TriageCard) []FileRef {
	var refs []FileRef
	seen := make(map[string]bool)

	for _, card := range cards {
		// Extract from main message
		for _, ref := range extractFromText(card.RawMessage, "message") {
			key := ref.Path + ":" + ref.Function
			if !seen[key] {
				seen[key] = true
				refs = append(refs, ref)
			}
		}

		// Extract from pre-context
		for _, line := range card.PreContext {
			for _, ref := range extractFromText(line, "context") {
				key := ref.Path + ":" + ref.Function
				if !seen[key] {
					seen[key] = true
					refs = append(refs, ref)
				}
			}
		}

		// Extract from post-context
		for _, line := range card.PostContext {
			for _, ref := range extractFromText(line, "context") {
				key := ref.Path + ":" + ref.Function
				if !seen[key] {
					seen[key] = true
					refs = append(refs, ref)
				}
			}
		}
	}

	return refs
}

// Common patterns for extracting file references from stack traces and error messages.
var fileRefPatterns = []*regexp.Regexp{
	// Python: File "path/to/file.py", line 123
	regexp.MustCompile(`File "([^"]+\.py)", line (\d+)`),
	// Python: path/to/file.py:123
	regexp.MustCompile(`([a-zA-Z0-9_/.-]+\.py):(\d+)`),
	// Go: path/to/file.go:123
	regexp.MustCompile(`([a-zA-Z0-9_/.-]+\.go):(\d+)`),
	// JavaScript/TypeScript: at func (path/to/file.js:123:45)
	regexp.MustCompile(`\(([a-zA-Z0-9_/.-]+\.[jt]sx?):(\d+):\d+\)`),
	// Generic: path/to/file.ext:123
	regexp.MustCompile(`([a-zA-Z0-9_/.-]+\.[a-zA-Z]+):(\d+)`),
	// Test file patterns: test_foo.py::TestClass::test_method
	regexp.MustCompile(`([a-zA-Z0-9_/.-]+\.py)::([a-zA-Z0-9_]+)::([a-zA-Z0-9_]+)`),
}

// Function name patterns
var funcPatterns = []*regexp.Regexp{
	// Python: in function_name
	regexp.MustCompile(`in ([a-zA-Z_][a-zA-Z0-9_]*)`),
	// Python: def function_name
	regexp.MustCompile(`def ([a-zA-Z_][a-zA-Z0-9_]*)`),
	// Go: func FunctionName
	regexp.MustCompile(`func ([a-zA-Z_][a-zA-Z0-9_]*)`),
	// Generic: FunctionName(
	regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*)\(`),
}

// extractFromText extracts file references from a text string.
func extractFromText(text, source string) []FileRef {
	var refs []FileRef

	for _, pattern := range fileRefPatterns {
		matches := pattern.FindAllStringSubmatch(text, -1)
		for _, match := range matches {
			if len(match) >= 2 {
				ref := FileRef{
					Path:   match[1],
					Source: source,
				}
				if len(match) >= 3 {
					// Try to parse line number
					var line int
					if _, err := parseIntSafe(match[2]); err == nil {
						ref.Line = line
					}
				}
				refs = append(refs, ref)
			}
		}
	}

	// Also extract function names
	for _, pattern := range funcPatterns {
		matches := pattern.FindAllStringSubmatch(text, -1)
		for _, match := range matches {
			if len(match) >= 2 {
				refs = append(refs, FileRef{
					Function: match[1],
					Source:   source,
				})
			}
		}
	}

	return refs
}

// matchFileRef scores how well a commit file matches a file reference.
func matchFileRef(cf githubactions.CommitFile, ref FileRef) (float64, string) {
	var score float64
	var evidence string

	// Exact path match
	if ref.Path != "" && cf.Filename == ref.Path {
		score = 1.0
		evidence = "exact path match: " + ref.Path
		return score, evidence
	}

	// Basename match
	if ref.Path != "" {
		refBase := baseName(ref.Path)
		cfBase := baseName(cf.Filename)
		if refBase == cfBase {
			score = 0.7
			evidence = "filename match: " + refBase
			return score, evidence
		}
	}

	// Partial path match (ref path is suffix of changed file)
	if ref.Path != "" && strings.HasSuffix(cf.Filename, ref.Path) {
		score = 0.8
		evidence = "partial path match: " + ref.Path
		return score, evidence
	}

	// Function name in patch
	if ref.Function != "" && cf.Patch != "" {
		if strings.Contains(cf.Patch, ref.Function) {
			score = 0.5
			evidence = "function '" + ref.Function + "' found in diff"
			return score, evidence
		}
	}

	// Test file naming convention: test_foo.py tests foo.py
	if ref.Path != "" {
		testName := baseName(ref.Path)
		if strings.HasPrefix(testName, "test_") {
			srcName := strings.TrimPrefix(testName, "test_")
			if baseName(cf.Filename) == srcName {
				score = 0.6
				evidence = "test naming convention: " + testName + " tests " + srcName
				return score, evidence
			}
		}
	}

	return 0, ""
}

// baseName returns the last component of a path.
func baseName(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

// parseIntSafe safely parses an int, returning an error if invalid.
func parseIntSafe(s string) (int, error) {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, nil
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}
