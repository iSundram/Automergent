// Package rootcause provides root cause analysis for diagnostic errors.
// It analyzes error chains, detects related files, and identifies dependency issues.
package rootcause

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/iSundram/Automergent/internal/diagnostics/compiler"
)

// Analysis represents the result of root cause analysis.
type Analysis struct {
	PrimaryError  *compiler.CompilerDiagnostic `json:"primary_error"`
	RootCause     string                       `json:"root_cause"`
	RootCauseType CauseType                    `json:"root_cause_type"`
	ErrorChain    []ChainedError               `json:"error_chain"`
	RelatedFiles  []RelatedFile                `json:"related_files"`
	Dependencies  []DependencyIssue            `json:"dependencies"`
	ConfigIssues  []ConfigIssue                `json:"config_issues"`
	Suggestions   []string                     `json:"suggestions"`
	Confidence    float64                      `json:"confidence"` // 0.0 to 1.0
}

// CauseType categorizes the type of root cause.
type CauseType string

// Root cause types.
const (
	CauseMissingImport     CauseType = "missing_import"
	CauseTypeError         CauseType = "type_error"
	CauseSyntaxError       CauseType = "syntax_error"
	CauseMissingDependency CauseType = "missing_dependency"
	CauseConfigError       CauseType = "config_error"
	CauseCircularDep       CauseType = "circular_dependency"
	CauseMissingFile       CauseType = "missing_file"
	CauseVersionMismatch   CauseType = "version_mismatch"
	CauseUnknown           CauseType = "unknown"
)

// ChainedError represents an error in an error chain.
type ChainedError struct {
	Diagnostic *compiler.CompilerDiagnostic `json:"diagnostic"`
	CausedBy   int                          `json:"caused_by"` // Index of the causing error (-1 if none)
	Depth      int                          `json:"depth"`     // Distance from root cause
}

// RelatedFile represents a file related to an error.
type RelatedFile struct {
	Path       string  `json:"path"`
	Relation   string  `json:"relation"` // e.g., "imports", "imported_by", "depends_on"
	Confidence float64 `json:"confidence"`
}

// DependencyIssue represents a dependency-related problem.
type DependencyIssue struct {
	Package    string `json:"package"`
	Expected   string `json:"expected_version,omitempty"`
	Actual     string `json:"actual_version,omitempty"`
	Issue      string `json:"issue"` // "missing", "version_mismatch", "circular"
	Resolution string `json:"resolution"`
}

// ConfigIssue represents a configuration-related problem.
type ConfigIssue struct {
	File       string `json:"file"`
	Key        string `json:"key,omitempty"`
	Issue      string `json:"issue"`
	Resolution string `json:"resolution"`
}

// Analyzer performs root cause analysis on diagnostics.
type Analyzer struct {
	diagnostics  []compiler.CompilerDiagnostic
	fileContents map[string]string // Cache of file contents
	projectRoot  string
	errorsByFile map[string][]int // Map of file paths to diagnostic indices
}

// NewAnalyzer creates a new root cause analyzer.
func NewAnalyzer(diagnostics []compiler.CompilerDiagnostic) *Analyzer {
	a := &Analyzer{
		diagnostics:  diagnostics,
		fileContents: make(map[string]string),
		errorsByFile: make(map[string][]int),
	}

	// Index errors by file
	for i, d := range diagnostics {
		a.errorsByFile[d.FilePath] = append(a.errorsByFile[d.FilePath], i)
	}

	return a
}

// SetProjectRoot sets the project root directory.
func (a *Analyzer) SetProjectRoot(root string) {
	a.projectRoot = root
}

// SetFileContent caches file content for analysis.
func (a *Analyzer) SetFileContent(path, content string) {
	a.fileContents[path] = content
}

// Analyze performs root cause analysis on all diagnostics.
func (a *Analyzer) Analyze() []Analysis {
	var results []Analysis

	// Group related errors
	errorGroups := a.groupRelatedErrors()

	for _, group := range errorGroups {
		analysis := a.analyzeGroup(group)
		results = append(results, analysis)
	}

	return results
}

// AnalyzeSingle performs root cause analysis on a single diagnostic.
func (a *Analyzer) AnalyzeSingle(diag *compiler.CompilerDiagnostic) Analysis {
	analysis := Analysis{
		PrimaryError: diag,
		Confidence:   0.5,
	}

	// Determine root cause type
	analysis.RootCauseType = a.determineRootCauseType(diag)
	analysis.RootCause = a.generateRootCauseDescription(diag, analysis.RootCauseType)

	// Find related files
	analysis.RelatedFiles = a.findRelatedFiles(diag)

	// Check for dependency issues
	analysis.Dependencies = a.checkDependencyIssues(diag)

	// Check for config issues
	analysis.ConfigIssues = a.checkConfigIssues(diag)

	// Generate suggestions
	analysis.Suggestions = a.generateSuggestions(diag, analysis.RootCauseType)

	// Calculate confidence
	analysis.Confidence = a.calculateConfidence(analysis)

	return analysis
}

// groupRelatedErrors groups diagnostics that are likely related.
func (a *Analyzer) groupRelatedErrors() [][]int {
	var groups [][]int
	visited := make(map[int]bool)

	for i := range a.diagnostics {
		if visited[i] {
			continue
		}

		group := a.findRelatedErrors(i, visited)
		if len(group) > 0 {
			groups = append(groups, group)
		}
	}

	return groups
}

// findRelatedErrors finds all errors related to a given error.
func (a *Analyzer) findRelatedErrors(startIdx int, visited map[int]bool) []int {
	var group []int
	queue := []int{startIdx}

	for len(queue) > 0 {
		idx := queue[0]
		queue = queue[1:]

		if visited[idx] {
			continue
		}
		visited[idx] = true
		group = append(group, idx)

		// Find errors in the same file
		diag := &a.diagnostics[idx]
		for _, relatedIdx := range a.errorsByFile[diag.FilePath] {
			if !visited[relatedIdx] {
				queue = append(queue, relatedIdx)
			}
		}

		// Find errors in related files
		for _, relFile := range diag.RelatedFiles {
			for _, relatedIdx := range a.errorsByFile[relFile] {
				if !visited[relatedIdx] {
					queue = append(queue, relatedIdx)
				}
			}
		}
	}

	return group
}

// analyzeGroup performs analysis on a group of related errors.
func (a *Analyzer) analyzeGroup(group []int) Analysis {
	if len(group) == 0 {
		return Analysis{}
	}

	// Find the primary/root error
	rootIdx := a.findRootError(group)
	primary := &a.diagnostics[rootIdx]

	analysis := a.AnalyzeSingle(primary)

	// Build error chain
	analysis.ErrorChain = a.buildErrorChain(group, rootIdx)

	return analysis
}

// findRootError finds the most likely root cause error in a group.
func (a *Analyzer) findRootError(group []int) int {
	// Priority order for root causes:
	// 1. Import/dependency errors
	// 2. Config errors
	// 3. Syntax errors
	// 4. Type errors
	// 5. Others

	type candidate struct {
		idx      int
		priority int
	}

	var candidates []candidate
	for _, idx := range group {
		diag := &a.diagnostics[idx]
		priority := 99

		switch diag.Category {
		case compiler.CategoryImport:
			priority = 1
		case compiler.CategoryDependency:
			priority = 2
		case compiler.CategoryConfig:
			priority = 3
		case compiler.CategorySyntax:
			priority = 4
		case compiler.CategoryType:
			priority = 5
		}

		candidates = append(candidates, candidate{idx, priority})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].priority < candidates[j].priority
	})

	if len(candidates) > 0 {
		return candidates[0].idx
	}
	return group[0]
}

// buildErrorChain builds the chain of errors.
func (a *Analyzer) buildErrorChain(group []int, rootIdx int) []ChainedError {
	var chain []ChainedError

	// Simple linear chain based on line order for now
	sort.Slice(group, func(i, j int) bool {
		di := &a.diagnostics[group[i]]
		dj := &a.diagnostics[group[j]]
		if di.FilePath != dj.FilePath {
			return di.FilePath < dj.FilePath
		}
		return di.Line < dj.Line
	})

	for i, idx := range group {
		causedBy := -1
		if i > 0 {
			causedBy = i - 1
		}

		depth := 0
		for _, ri := range group {
			if ri == rootIdx {
				break
			}
			depth++
		}

		chain = append(chain, ChainedError{
			Diagnostic: &a.diagnostics[idx],
			CausedBy:   causedBy,
			Depth:      depth,
		})
	}

	return chain
}

// determineRootCauseType determines the type of root cause.
func (a *Analyzer) determineRootCauseType(diag *compiler.CompilerDiagnostic) CauseType {
	msg := strings.ToLower(diag.Message)

	switch {
	case diag.Category == compiler.CategoryImport:
		return CauseMissingImport
	case diag.Category == compiler.CategorySyntax:
		return CauseSyntaxError
	case diag.Category == compiler.CategoryType:
		return CauseTypeError
	case diag.Category == compiler.CategoryConfig:
		return CauseConfigError
	case strings.Contains(msg, "dependency") || strings.Contains(msg, "not found"):
		return CauseMissingDependency
	case strings.Contains(msg, "circular"):
		return CauseCircularDep
	case strings.Contains(msg, "version"):
		return CauseVersionMismatch
	case strings.Contains(msg, "file not found") || strings.Contains(msg, "no such file"):
		return CauseMissingFile
	default:
		return CauseUnknown
	}
}

// generateRootCauseDescription generates a human-readable description.
func (a *Analyzer) generateRootCauseDescription(diag *compiler.CompilerDiagnostic, causeType CauseType) string {
	switch causeType {
	case CauseMissingImport:
		return a.describeImportIssue(diag)
	case CauseSyntaxError:
		return a.describeSyntaxIssue(diag)
	case CauseTypeError:
		return a.describeTypeIssue(diag)
	case CauseMissingDependency:
		return a.describeDependencyIssue(diag)
	case CauseConfigError:
		return a.describeConfigIssue(diag)
	case CauseCircularDep:
		return "Circular dependency detected in import chain"
	case CauseVersionMismatch:
		return "Version mismatch between required and installed dependency"
	case CauseMissingFile:
		return "Required file does not exist"
	default:
		return diag.Message
	}
}

func (a *Analyzer) describeImportIssue(diag *compiler.CompilerDiagnostic) string {
	// Extract package name from message
	patterns := []string{
		`cannot find package "([^"]+)"`,
		`cannot find module '([^']+)'`,
		`Module not found: '([^']+)'`,
		`No module named '([^']+)'`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(diag.Message); matches != nil {
			return "Missing import: " + matches[1]
		}
	}

	return "Import/module resolution failure"
}

func (a *Analyzer) describeSyntaxIssue(diag *compiler.CompilerDiagnostic) string {
	msg := strings.ToLower(diag.Message)

	switch {
	case strings.Contains(msg, "unexpected"):
		return "Unexpected token in source code"
	case strings.Contains(msg, "missing"):
		return "Missing token or delimiter"
	case strings.Contains(msg, "unclosed"):
		return "Unclosed bracket, string, or block"
	default:
		return "Syntax error in source code"
	}
}

func (a *Analyzer) describeTypeIssue(diag *compiler.CompilerDiagnostic) string {
	msg := strings.ToLower(diag.Message)

	switch {
	case strings.Contains(msg, "undefined"):
		return "Reference to undefined variable or function"
	case strings.Contains(msg, "cannot use") || strings.Contains(msg, "incompatible"):
		return "Type mismatch in assignment or function call"
	case strings.Contains(msg, "not assignable"):
		return "Value cannot be assigned to the target type"
	default:
		return "Type system violation"
	}
}

func (a *Analyzer) describeDependencyIssue(diag *compiler.CompilerDiagnostic) string {
	return "Missing or incompatible dependency"
}

func (a *Analyzer) describeConfigIssue(diag *compiler.CompilerDiagnostic) string {
	return "Configuration file error"
}

// findRelatedFiles finds files related to the diagnostic.
func (a *Analyzer) findRelatedFiles(diag *compiler.CompilerDiagnostic) []RelatedFile {
	var files []RelatedFile

	// Add explicitly mentioned related files
	for _, f := range diag.RelatedFiles {
		files = append(files, RelatedFile{
			Path:       f,
			Relation:   "mentioned",
			Confidence: 0.9,
		})
	}

	// Extract file references from the message
	filePatterns := []string{
		`"([^"]+\.(go|py|ts|js|rs|java))"`,
		`'([^']+\.(go|py|ts|js|rs|java))'`,
		`([A-Za-z0-9_./]+\.(go|py|ts|js|rs|java))`,
	}

	for _, pattern := range filePatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllStringSubmatch(diag.Message, -1)
		for _, match := range matches {
			if match[1] != diag.FilePath {
				files = append(files, RelatedFile{
					Path:       match[1],
					Relation:   "referenced",
					Confidence: 0.7,
				})
			}
		}
	}

	// Check for import relationships from context
	for _, ctx := range diag.Context {
		for _, pattern := range filePatterns {
			re := regexp.MustCompile(pattern)
			matches := re.FindAllStringSubmatch(ctx, -1)
			for _, match := range matches {
				if match[1] != diag.FilePath {
					files = append(files, RelatedFile{
						Path:       match[1],
						Relation:   "context",
						Confidence: 0.5,
					})
				}
			}
		}
	}

	return dedupRelatedFiles(files)
}

func dedupRelatedFiles(files []RelatedFile) []RelatedFile {
	seen := make(map[string]bool)
	var result []RelatedFile

	for _, f := range files {
		if !seen[f.Path] {
			seen[f.Path] = true
			result = append(result, f)
		}
	}

	return result
}

// checkDependencyIssues checks for dependency-related problems.
func (a *Analyzer) checkDependencyIssues(diag *compiler.CompilerDiagnostic) []DependencyIssue {
	var issues []DependencyIssue

	// Check for missing package patterns
	missingPkgPatterns := []struct {
		pattern    string
		resolution string
	}{
		{`cannot find package "([^"]+)"`, "Run: go get %s"},
		{`Module not found: .* '([^']+)'`, "Run: npm install %s"},
		{`No module named '([^']+)'`, "Run: pip install %s"},
		{`could not find crate '([^']+)'`, "Add to Cargo.toml: %s"},
	}

	for _, p := range missingPkgPatterns {
		re := regexp.MustCompile(p.pattern)
		if matches := re.FindStringSubmatch(diag.Message); matches != nil {
			issues = append(issues, DependencyIssue{
				Package:    matches[1],
				Issue:      "missing",
				Resolution: strings.Replace(p.resolution, "%s", matches[1], 1),
			})
		}
	}

	return issues
}

// checkConfigIssues checks for configuration-related problems.
func (a *Analyzer) checkConfigIssues(diag *compiler.CompilerDiagnostic) []ConfigIssue {
	var issues []ConfigIssue

	// Check file extension for config files
	ext := strings.ToLower(filepath.Ext(diag.FilePath))
	configExtensions := map[string]string{
		".json":      "JSON configuration file",
		".yaml":      "YAML configuration file",
		".yml":       "YAML configuration file",
		".toml":      "TOML configuration file",
		".ini":       "INI configuration file",
		"tsconfig":   "TypeScript configuration",
		"package":    "NPM package configuration",
		"go.mod":     "Go module configuration",
		"cargo.toml": "Cargo configuration",
	}

	base := strings.ToLower(filepath.Base(diag.FilePath))
	for pattern, desc := range configExtensions {
		if ext == pattern || strings.Contains(base, pattern) {
			issues = append(issues, ConfigIssue{
				File:       diag.FilePath,
				Issue:      desc + " error",
				Resolution: "Check " + diag.FilePath + " for syntax errors",
			})
			break
		}
	}

	return issues
}

// generateSuggestions generates fix suggestions.
func (a *Analyzer) generateSuggestions(diag *compiler.CompilerDiagnostic, causeType CauseType) []string {
	var suggestions []string

	// Add compiler-provided suggestions first
	suggestions = append(suggestions, diag.Suggestions...)

	// Add context-aware suggestions
	switch causeType {
	case CauseMissingImport:
		suggestions = append(suggestions,
			"Check the import path spelling",
			"Verify the package is installed",
			"Check if the package name has changed",
		)
	case CauseSyntaxError:
		suggestions = append(suggestions,
			"Check for missing brackets, parentheses, or semicolons",
			"Verify indentation (for Python)",
			"Check for unclosed strings or comments",
		)
	case CauseTypeError:
		suggestions = append(suggestions,
			"Check variable declarations",
			"Verify function signatures match",
			"Check for typos in variable names",
		)
	case CauseMissingDependency:
		suggestions = append(suggestions,
			"Install missing dependencies",
			"Update dependency lock file",
			"Check for version conflicts",
		)
	case CauseConfigError:
		suggestions = append(suggestions,
			"Validate configuration file syntax",
			"Check for required fields",
			"Verify file paths in configuration",
		)
	}

	return suggestions
}

// calculateConfidence calculates confidence score for the analysis.
func (a *Analyzer) calculateConfidence(analysis Analysis) float64 {
	score := 0.5 // Base confidence

	// Increase for explicit error codes
	if analysis.PrimaryError != nil && analysis.PrimaryError.ErrorCode != "" {
		score += 0.1
	}

	// Increase for known cause types
	if analysis.RootCauseType != CauseUnknown {
		score += 0.15
	}

	// Increase for compiler suggestions
	if analysis.PrimaryError != nil && len(analysis.PrimaryError.Suggestions) > 0 {
		score += 0.1
	}

	// Increase for detected related files
	if len(analysis.RelatedFiles) > 0 {
		score += 0.05
	}

	// Increase for detected dependency issues
	if len(analysis.Dependencies) > 0 {
		score += 0.1
	}

	// Cap at 1.0
	if score > 1.0 {
		score = 1.0
	}

	return score
}

// AnalyzeCompilerOutput is a convenience function that parses and analyzes compiler output.
func AnalyzeCompilerOutput(output string, lang compiler.Language) []Analysis {
	diags := compiler.ParseOutput(output, lang)
	analyzer := NewAnalyzer(diags)
	return analyzer.Analyze()
}
