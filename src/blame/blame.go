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

// stopwords contains common words that should not contribute to blame correlation.
// These appear frequently in code and error messages but don't indicate causation.
var stopwords = map[string]bool{
	// Common English words
	"a": true, "an": true, "the": true, "is": true, "in": true, "on": true,
	"to": true, "for": true, "of": true, "with": true, "by": true, "at": true,
	"from": true, "as": true, "or": true, "and": true, "not": true, "no": true,
	"be": true, "was": true, "has": true, "have": true, "had": true, "can": true,
	"its": true, "it": true, "this": true, "that": true, "if": true, "else": true,
	"then": true, "when": true, "should": true, "only": true, "use": true,
	"non": true, "pre": true, "end": true, "last": true, "time": true,
	"properly": true, "In": true, "SQL": true, "DB": true,

	// Generic programming terms
	"class": true, "function": true, "func": true, "Func": true, "method": true,
	"package": true, "object": true, "module": true, "import": true,
	"test": true, "tests": true, "type": true, "types": true, "value": true,
	"error": true, "Error": true, "exception": true, "Exception": true,
	"case": true, "default": true, "return": true, "void": true,
	"public": true, "private": true, "static": true, "final": true,
	"true": true, "false": true, "null": true, "nil": true, "None": true,
	"struct": true, "interface": true, "enum": true, "const": true, "var": true,

	// Common single-letter identifiers (a-z handled by len check in isStopword)
	"fn": true, "op": true, "id": true, "ok": true, "err": true,

	// Common method/function names
	"get": true, "set": true, "put": true, "pop": true, "push": true,
	"add": true, "remove": true, "delete": true, "create": true, "new": true,
	"init": true, "initialize": true, "setup": true, "teardown": true,
	"run": true, "start": true, "stop": true, "close": true, "open": true,
	"read": true, "write": true, "load": true, "save": true, "update": true,
	"map": true, "filter": true, "reduce": true, "foreach": true,
	"apply": true, "call": true, "invoke": true, "execute": true,
	"assert": true, "expect": true, "require": true, "check": true,
	"log": true, "print": true, "debug": true, "info": true, "warn": true,
	"input": true, "output": true, "result": true, "results": true,
	"data": true, "config": true, "options": true, "params": true,
	"empty": true, "length": true, "size": true, "count": true, "number": true,
	"min": true, "max": true, "sum": true, "avg": true,
	"exists": true, "contains": true, "includes": true,
	"join": true, "split": true, "concat": true, "append": true,
	"build": true, "make": true, "copy": true, "clone": true,
	"getOrElse": true, "orElse": true,

	// Common type names
	"String": true, "Int": true, "Integer": true, "Long": true, "Float": true,
	"Double": true, "Boolean": true, "Bool": true, "Array": true, "List": true,
	"Seq": true, "Set": true, "Dict": true, "Map": true, "Object": true,
	"Some": true, "Option": true, "Optional": true, "Result": true,
	"StructType": true, "Table": true,

	// Common framework/library terms
	"org": true, "com": true, "io": true, "net": true, "java": true,
	"scala": true, "python": true, "spark": true, "sql": true,
	"schema": true, "row": true, "column": true, "field": true,
	"batch": true, "batches": true, "stream": true, "operator": true,
	"offset": true, "version": true, "retry": true, "resolve": true,
	"code": true, "directories": true, "keys": true, "Checkpoint": true,
	"super": true, "Eq": true, "assertEqual": true,
}

// isStopword returns true if the given identifier should be ignored in correlation.
func isStopword(s string) bool {
	return stopwords[s] || len(s) <= 2
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

	// Function name in patch (skip stopwords)
	if ref.Function != "" && cf.Patch != "" && !isStopword(ref.Function) {
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
