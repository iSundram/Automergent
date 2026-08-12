package git

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// CommitType represents conventional commit types
type CommitType string

const (
	CommitTypeFeat     CommitType = "feat"
	CommitTypeFix      CommitType = "fix"
	CommitTypeDocs     CommitType = "docs"
	CommitTypeStyle    CommitType = "style"
	CommitTypeRefactor CommitType = "refactor"
	CommitTypePerf     CommitType = "perf"
	CommitTypeTest     CommitType = "test"
	CommitTypeBuild    CommitType = "build"
	CommitTypeCI       CommitType = "ci"
	CommitTypeChore    CommitType = "chore"
	CommitTypeRevert   CommitType = "revert"
)

// SemanticCommit represents a conventional commit message
type SemanticCommit struct {
	Type           CommitType
	Scope          string
	Description    string
	Body           string
	Footer         string
	BreakingChange bool
	Issues         []string
}

// ChangeAnalysis contains analysis of code changes
type ChangeAnalysis struct {
	Files           []FileChange
	PrimaryType     CommitType
	Scope           string
	Summary         string
	BreakingChanges []string
	AffectedAreas   []string
}

// FileChange represents changes to a single file
type FileChange struct {
	Path       string
	Status     string // A=added, M=modified, D=deleted, R=renamed
	Additions  int
	Deletions  int
	ChangeType CommitType
	Category   string
}

// ChangeGroup represents a group of related changes
type ChangeGroup struct {
	Type        CommitType
	Scope       string
	Files       []FileChange
	Description string
}

// AnalyzeChanges analyzes staged changes and determines intent
func AnalyzeChanges(ctx context.Context, dir string) (*ChangeAnalysis, error) {
	// Get staged files with stats
	files, err := getStagedFiles(ctx, dir)
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		// Fall back to unstaged changes
		files, err = getUnstagedFiles(ctx, dir)
		if err != nil {
			return nil, err
		}
	}

	analysis := &ChangeAnalysis{
		Files: files,
	}

	// Analyze each file to determine change type
	for i := range analysis.Files {
		analysis.Files[i].ChangeType = categorizeFile(analysis.Files[i].Path)
		analysis.Files[i].Category = getFileCategory(analysis.Files[i].Path)
	}

	// Determine primary commit type
	analysis.PrimaryType = determinePrimaryType(files)

	// Extract scope from file paths
	analysis.Scope = extractScope(files)

	// Generate summary
	analysis.Summary = generateChangeSummary(files)

	// Detect breaking changes
	analysis.BreakingChanges = detectBreakingChanges(ctx, dir, files)

	// Identify affected areas
	analysis.AffectedAreas = identifyAffectedAreas(files)

	return analysis, nil
}

// GenerateCommitMessage generates a semantic commit message
func GenerateCommitMessage(analysis *ChangeAnalysis) *SemanticCommit {
	commit := &SemanticCommit{
		Type:           analysis.PrimaryType,
		Scope:          analysis.Scope,
		Description:    analysis.Summary,
		BreakingChange: len(analysis.BreakingChanges) > 0,
	}

	// Build body with details
	if len(analysis.Files) > 1 {
		var bodyParts []string
		for _, file := range analysis.Files {
			action := getActionVerb(file.Status)
			bodyParts = append(bodyParts, fmt.Sprintf("- %s %s", action, file.Path))
		}
		commit.Body = strings.Join(bodyParts, "\n")
	}

	// Add breaking change footer if applicable
	if commit.BreakingChange {
		commit.Footer = "BREAKING CHANGE: " + strings.Join(analysis.BreakingChanges, "; ")
	}

	return commit
}

// Format returns the formatted commit message
func (c *SemanticCommit) Format() string {
	var sb strings.Builder

	// Type and scope
	if c.BreakingChange {
		if c.Scope != "" {
			sb.WriteString(fmt.Sprintf("%s(%s)!: %s", c.Type, c.Scope, c.Description))
		} else {
			sb.WriteString(fmt.Sprintf("%s!: %s", c.Type, c.Description))
		}
	} else {
		if c.Scope != "" {
			sb.WriteString(fmt.Sprintf("%s(%s): %s", c.Type, c.Scope, c.Description))
		} else {
			sb.WriteString(fmt.Sprintf("%s: %s", c.Type, c.Description))
		}
	}

	// Body
	if c.Body != "" {
		sb.WriteString("\n\n")
		sb.WriteString(c.Body)
	}

	// Footer
	if c.Footer != "" {
		sb.WriteString("\n\n")
		sb.WriteString(c.Footer)
	}

	// Issues
	if len(c.Issues) > 0 {
		sb.WriteString("\n\n")
		for _, issue := range c.Issues {
			sb.WriteString(fmt.Sprintf("Closes #%s\n", issue))
		}
	}

	return sb.String()
}

// GroupChanges groups related changes for multi-commit workflows
func GroupChanges(ctx context.Context, dir string) ([]ChangeGroup, error) {
	analysis, err := AnalyzeChanges(ctx, dir)
	if err != nil {
		return nil, err
	}

	// Group files by category and type
	groups := make(map[string]*ChangeGroup)

	for _, file := range analysis.Files {
		key := fmt.Sprintf("%s-%s", file.ChangeType, file.Category)
		if g, ok := groups[key]; ok {
			g.Files = append(g.Files, file)
		} else {
			groups[key] = &ChangeGroup{
				Type:  file.ChangeType,
				Scope: file.Category,
				Files: []FileChange{file},
			}
		}
	}

	// Convert to slice and generate descriptions
	result := make([]ChangeGroup, 0, len(groups))
	for _, g := range groups {
		g.Description = generateGroupDescription(g)
		result = append(result, *g)
	}

	// Sort by type priority
	sort.Slice(result, func(i, j int) bool {
		return commitTypePriority(result[i].Type) < commitTypePriority(result[j].Type)
	})

	return result, nil
}

// Helper functions

func getStagedFiles(ctx context.Context, dir string) ([]FileChange, error) {
	out, err := runGit(ctx, dir, "diff", "--cached", "--numstat", "--name-status")
	if err != nil {
		return nil, err
	}
	return parseFileStats(out), nil
}

func getUnstagedFiles(ctx context.Context, dir string) ([]FileChange, error) {
	out, err := runGit(ctx, dir, "diff", "--numstat", "--name-status")
	if err != nil {
		return nil, err
	}
	return parseFileStats(out), nil
}

func parseFileStats(output string) []FileChange {
	var files []FileChange
	lines := strings.Split(strings.TrimSpace(output), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			fc := FileChange{
				Status: string(parts[0][0]),
				Path:   parts[len(parts)-1],
			}
			// Parse additions/deletions if available
			if len(parts) >= 3 {
				fmt.Sscanf(parts[0], "%d", &fc.Additions)
				fmt.Sscanf(parts[1], "%d", &fc.Deletions)
			}
			files = append(files, fc)
		}
	}
	return files
}

func categorizeFile(path string) CommitType {
	path = strings.ToLower(path)

	// Test files
	if strings.Contains(path, "_test.") || strings.Contains(path, ".test.") ||
		strings.Contains(path, "/test/") || strings.Contains(path, "/tests/") {
		return CommitTypeTest
	}

	// Documentation
	if strings.HasSuffix(path, ".md") || strings.HasSuffix(path, ".rst") ||
		strings.HasSuffix(path, ".txt") || strings.Contains(path, "/docs/") {
		return CommitTypeDocs
	}

	// Build/CI files
	if strings.Contains(path, "makefile") || strings.Contains(path, "dockerfile") ||
		strings.Contains(path, ".yml") || strings.Contains(path, ".yaml") ||
		strings.Contains(path, "go.mod") || strings.Contains(path, "go.sum") ||
		strings.Contains(path, "package.json") || strings.Contains(path, ".github/") {
		if strings.Contains(path, ".github/") {
			return CommitTypeCI
		}
		return CommitTypeBuild
	}

	// Style files
	if strings.HasSuffix(path, ".css") || strings.HasSuffix(path, ".scss") ||
		strings.HasSuffix(path, ".less") {
		return CommitTypeStyle
	}

	// Default to feat for new functionality
	return CommitTypeFeat
}

func getFileCategory(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) > 1 {
		// Use second-level directory as scope
		if parts[0] == "internal" || parts[0] == "pkg" || parts[0] == "cmd" {
			if len(parts) > 2 {
				return parts[1]
			}
		}
		return parts[0]
	}
	return ""
}

func determinePrimaryType(files []FileChange) CommitType {
	typeCounts := make(map[CommitType]int)
	for _, f := range files {
		typeCounts[f.ChangeType]++
	}

	// Priority order for type selection
	priority := []CommitType{
		CommitTypeFix,
		CommitTypeFeat,
		CommitTypeRefactor,
		CommitTypePerf,
		CommitTypeTest,
		CommitTypeDocs,
		CommitTypeBuild,
		CommitTypeCI,
		CommitTypeStyle,
		CommitTypeChore,
	}

	for _, t := range priority {
		if typeCounts[t] > 0 {
			return t
		}
	}
	return CommitTypeChore
}

func extractScope(files []FileChange) string {
	scopes := make(map[string]int)
	for _, f := range files {
		scope := getFileCategory(f.Path)
		if scope != "" {
			scopes[scope]++
		}
	}

	// Find most common scope
	var maxScope string
	var maxCount int
	for scope, count := range scopes {
		if count > maxCount {
			maxScope = scope
			maxCount = count
		}
	}

	// Only use scope if it represents majority of changes
	if maxCount > len(files)/2 {
		return maxScope
	}
	return ""
}

func generateChangeSummary(files []FileChange) string {
	if len(files) == 0 {
		return "no changes"
	}

	if len(files) == 1 {
		action := getActionVerb(files[0].Status)
		return fmt.Sprintf("%s %s", action, getFilename(files[0].Path))
	}

	// Multiple files - group by action
	actions := make(map[string]int)
	for _, f := range files {
		actions[f.Status]++
	}

	var parts []string
	if n := actions["A"]; n > 0 {
		parts = append(parts, fmt.Sprintf("add %d files", n))
	}
	if n := actions["M"]; n > 0 {
		parts = append(parts, fmt.Sprintf("update %d files", n))
	}
	if n := actions["D"]; n > 0 {
		parts = append(parts, fmt.Sprintf("remove %d files", n))
	}

	return strings.Join(parts, ", ")
}

func detectBreakingChanges(ctx context.Context, dir string, files []FileChange) []string {
	var breaking []string

	for _, f := range files {
		// Deleted public API files
		if f.Status == "D" && isPublicAPI(f.Path) {
			breaking = append(breaking, fmt.Sprintf("removed %s", f.Path))
		}

		// Check for signature changes in public functions
		if f.Status == "M" && isPublicAPI(f.Path) {
			diff, _ := runGit(ctx, dir, "diff", "--cached", "--", f.Path)
			if containsSignatureChange(diff) {
				breaking = append(breaking, fmt.Sprintf("API changes in %s", f.Path))
			}
		}
	}

	return breaking
}

func isPublicAPI(path string) bool {
	// Go: exported functions in pkg/
	if strings.HasPrefix(path, "pkg/") && strings.HasSuffix(path, ".go") {
		return true
	}
	// API definitions
	if strings.Contains(path, "/api/") {
		return true
	}
	return false
}

func containsSignatureChange(diff string) bool {
	// Look for removed function signatures
	patterns := []string{
		`^-\s*func\s+\([^)]+\)\s+\w+`,          // method signature
		`^-\s*func\s+\w+`,                      // function signature
		`^-\s*type\s+\w+\s+(struct|interface)`, // type definition
	}

	for _, p := range patterns {
		if matched, _ := regexp.MatchString(p, diff); matched {
			return true
		}
	}
	return false
}

func identifyAffectedAreas(files []FileChange) []string {
	areas := make(map[string]bool)
	for _, f := range files {
		category := getFileCategory(f.Path)
		if category != "" {
			areas[category] = true
		}
	}

	result := make([]string, 0, len(areas))
	for area := range areas {
		result = append(result, area)
	}
	sort.Strings(result)
	return result
}

func getActionVerb(status string) string {
	switch status {
	case "A":
		return "add"
	case "M":
		return "update"
	case "D":
		return "remove"
	case "R":
		return "rename"
	case "C":
		return "copy"
	default:
		return "modify"
	}
}

func getFilename(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

func generateGroupDescription(g *ChangeGroup) string {
	if len(g.Files) == 1 {
		return fmt.Sprintf("%s %s", getActionVerb(g.Files[0].Status), getFilename(g.Files[0].Path))
	}
	return fmt.Sprintf("%s %d %s files", getActionVerb(g.Files[0].Status), len(g.Files), g.Scope)
}

func commitTypePriority(t CommitType) int {
	priorities := map[CommitType]int{
		CommitTypeFix:      1,
		CommitTypeFeat:     2,
		CommitTypeRefactor: 3,
		CommitTypePerf:     4,
		CommitTypeTest:     5,
		CommitTypeDocs:     6,
		CommitTypeBuild:    7,
		CommitTypeCI:       8,
		CommitTypeStyle:    9,
		CommitTypeChore:    10,
		CommitTypeRevert:   11,
	}
	if p, ok := priorities[t]; ok {
		return p
	}
	return 100
}
