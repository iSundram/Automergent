package git

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// DiffAnalysis provides detailed analysis of changes
type DiffAnalysis struct {
	Files       []FileDiff
	Summary     *DiffSummary
	Impact      *ImpactAssessment
	Categories  map[string][]FileDiff
	ReviewNotes []string
}

// FileDiff represents detailed diff for a single file
type FileDiff struct {
	Path       string
	Status     string // A, M, D, R
	OldPath    string // For renames
	Hunks      []DiffHunk
	Additions  int
	Deletions  int
	Binary     bool
	Language   string
	Category   string
	Complexity string // "trivial", "simple", "moderate", "complex"
}

// DiffHunk represents a single hunk in a diff
type DiffHunk struct {
	OldStart   int
	OldLines   int
	NewStart   int
	NewLines   int
	Header     string
	Lines      []DiffLine
	ChangeType string // "addition", "deletion", "modification", "refactor"
}

// DiffLine represents a single line in a diff
type DiffLine struct {
	Type       string // "+", "-", " "
	Content    string
	OldLineNum int
	NewLineNum int
}

// DiffSummary provides an overview of changes
type DiffSummary struct {
	FilesChanged   int
	TotalAdditions int
	TotalDeletions int
	NetChange      int
	Languages      []string
	Categories     []string
	ScopeSize      string // "small", "medium", "large", "major"
}

// ImpactAssessment evaluates the impact of changes
type ImpactAssessment struct {
	RiskLevel           string // "low", "medium", "high"
	TestingRequired     bool
	ReviewRequired      bool
	DocumentationImpact bool
	APIChanges          bool
	BreakingChanges     bool
	SecurityRelevant    bool
	Concerns            []string
	Recommendations     []string
}

// AnalyzeDiff performs detailed diff analysis
func AnalyzeDiff(ctx context.Context, dir string, staged bool) (*DiffAnalysis, error) {
	var err error

	if staged {
		_, err = runGit(ctx, dir, "diff", "--cached", "-U3", "--stat")
	} else {
		_, err = runGit(ctx, dir, "diff", "-U3", "--stat")
	}
	if err != nil {
		return nil, err
	}

	analysis := &DiffAnalysis{
		Categories: make(map[string][]FileDiff),
	}

	// Parse diff output
	analysis.Files = parseDiffFiles(ctx, dir, staged)

	// Categorize files
	for i := range analysis.Files {
		f := &analysis.Files[i]
		f.Category = categorizeFile(f.Path).String()
		f.Language = detectLanguage(f.Path)
		f.Complexity = assessFileComplexity(f)

		analysis.Categories[f.Category] = append(analysis.Categories[f.Category], *f)
	}

	// Build summary
	analysis.Summary = buildDiffSummary(analysis.Files)

	// Assess impact
	analysis.Impact = assessImpact(analysis)

	// Generate review notes
	analysis.ReviewNotes = generateReviewNotes(analysis)

	return analysis, nil
}

// AnalyzeDiffBetween analyzes diff between two refs
func AnalyzeDiffBetween(ctx context.Context, dir, from, to string) (*DiffAnalysis, error) {
	diffOutput, err := runGit(ctx, dir, "diff", from+"..."+to, "-U3")
	if err != nil {
		return nil, err
	}

	analysis := &DiffAnalysis{
		Categories: make(map[string][]FileDiff),
	}

	analysis.Files = parseDiffOutput(diffOutput)

	for i := range analysis.Files {
		f := &analysis.Files[i]
		f.Category = categorizeFile(f.Path).String()
		f.Language = detectLanguage(f.Path)
		f.Complexity = assessFileComplexity(f)
		analysis.Categories[f.Category] = append(analysis.Categories[f.Category], *f)
	}

	analysis.Summary = buildDiffSummary(analysis.Files)
	analysis.Impact = assessImpact(analysis)
	analysis.ReviewNotes = generateReviewNotes(analysis)

	return analysis, nil
}

// GetSmartDiff returns a summarized, intelligent diff
func GetSmartDiff(ctx context.Context, dir string, staged bool) (string, error) {
	analysis, err := AnalyzeDiff(ctx, dir, staged)
	if err != nil {
		return "", err
	}

	return formatSmartDiff(analysis), nil
}

// PrepareForReview generates review-ready diff information
func PrepareForReview(ctx context.Context, dir string) (*ReviewPackage, error) {
	analysis, err := AnalyzeDiff(ctx, dir, true)
	if err != nil {
		return nil, err
	}

	pkg := &ReviewPackage{
		Summary:         analysis.Summary,
		Impact:          analysis.Impact,
		FilesByArea:     make(map[string][]string),
		KeyChanges:      identifyKeyChanges(analysis),
		TestsIncluded:   hasTestChanges(analysis),
		DocsIncluded:    hasDocChanges(analysis),
		ReviewChecklist: generateChecklist(analysis),
	}

	for category, files := range analysis.Categories {
		for _, f := range files {
			pkg.FilesByArea[category] = append(pkg.FilesByArea[category], f.Path)
		}
	}

	return pkg, nil
}

// ReviewPackage contains information for code review
type ReviewPackage struct {
	Summary         *DiffSummary
	Impact          *ImpactAssessment
	FilesByArea     map[string][]string
	KeyChanges      []string
	TestsIncluded   bool
	DocsIncluded    bool
	ReviewChecklist []string
}

// Helper functions

func parseDiffFiles(ctx context.Context, dir string, staged bool) []FileDiff {
	var args []string
	if staged {
		args = []string{"diff", "--cached", "--numstat"}
	} else {
		args = []string{"diff", "--numstat"}
	}

	out, _ := runGit(ctx, dir, args...)
	var files []FileDiff

	for _, line := range strings.Split(out, "\n") {
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}

		f := FileDiff{
			Path: parts[2],
		}

		// Handle binary files
		if parts[0] == "-" {
			f.Binary = true
		} else {
			fmt.Sscanf(parts[0], "%d", &f.Additions)
			fmt.Sscanf(parts[1], "%d", &f.Deletions)
		}

		// Detect status
		f.Status = "M"
		statusOut, _ := runGit(ctx, dir, "diff", "--cached", "--name-status", "--", f.Path)
		if statusOut != "" {
			f.Status = string(statusOut[0])
		}

		files = append(files, f)
	}

	return files
}

func parseDiffOutput(output string) []FileDiff {
	var files []FileDiff
	var current *FileDiff

	fileRe := regexp.MustCompile(`^diff --git a/(.+) b/(.+)`)
	hunkRe := regexp.MustCompile(`^@@ -(\d+),?(\d*) \+(\d+),?(\d*) @@(.*)`)

	for _, line := range strings.Split(output, "\n") {
		if matches := fileRe.FindStringSubmatch(line); matches != nil {
			if current != nil {
				files = append(files, *current)
			}
			current = &FileDiff{
				Path: matches[2],
			}
			if matches[1] != matches[2] {
				current.OldPath = matches[1]
				current.Status = "R"
			} else {
				current.Status = "M"
			}
		} else if current != nil {
			if strings.HasPrefix(line, "new file") {
				current.Status = "A"
			} else if strings.HasPrefix(line, "deleted file") {
				current.Status = "D"
			} else if strings.HasPrefix(line, "Binary") {
				current.Binary = true
			} else if matches := hunkRe.FindStringSubmatch(line); matches != nil {
				hunk := DiffHunk{
					Header: matches[5],
				}
				fmt.Sscanf(matches[1], "%d", &hunk.OldStart)
				fmt.Sscanf(matches[2], "%d", &hunk.OldLines)
				fmt.Sscanf(matches[3], "%d", &hunk.NewStart)
				fmt.Sscanf(matches[4], "%d", &hunk.NewLines)
				current.Hunks = append(current.Hunks, hunk)
			} else if len(line) > 0 && (line[0] == '+' || line[0] == '-') {
				if line[0] == '+' && !strings.HasPrefix(line, "+++") {
					current.Additions++
				} else if line[0] == '-' && !strings.HasPrefix(line, "---") {
					current.Deletions++
				}
			}
		}
	}

	if current != nil {
		files = append(files, *current)
	}

	return files
}

func detectLanguage(path string) string {
	ext := strings.ToLower(path[strings.LastIndex(path, ".")+1:])
	langs := map[string]string{
		"go":    "Go",
		"js":    "JavaScript",
		"ts":    "TypeScript",
		"py":    "Python",
		"rb":    "Ruby",
		"java":  "Java",
		"rs":    "Rust",
		"c":     "C",
		"cpp":   "C++",
		"cs":    "C#",
		"php":   "PHP",
		"swift": "Swift",
		"kt":    "Kotlin",
		"md":    "Markdown",
		"yaml":  "YAML",
		"yml":   "YAML",
		"json":  "JSON",
		"toml":  "TOML",
		"sh":    "Shell",
		"bash":  "Shell",
	}

	if lang, ok := langs[ext]; ok {
		return lang
	}
	return "Unknown"
}

func assessFileComplexity(f *FileDiff) string {
	changes := f.Additions + f.Deletions

	if changes <= 5 {
		return "trivial"
	} else if changes <= 20 {
		return "simple"
	} else if changes <= 100 {
		return "moderate"
	}
	return "complex"
}

func buildDiffSummary(files []FileDiff) *DiffSummary {
	summary := &DiffSummary{
		FilesChanged: len(files),
	}

	langSet := make(map[string]bool)
	catSet := make(map[string]bool)

	for _, f := range files {
		summary.TotalAdditions += f.Additions
		summary.TotalDeletions += f.Deletions
		if f.Language != "Unknown" {
			langSet[f.Language] = true
		}
		catSet[f.Category] = true
	}

	summary.NetChange = summary.TotalAdditions - summary.TotalDeletions

	for lang := range langSet {
		summary.Languages = append(summary.Languages, lang)
	}
	for cat := range catSet {
		summary.Categories = append(summary.Categories, cat)
	}

	// Determine scope size
	totalChanges := summary.TotalAdditions + summary.TotalDeletions
	if totalChanges <= 50 {
		summary.ScopeSize = "small"
	} else if totalChanges <= 200 {
		summary.ScopeSize = "medium"
	} else if totalChanges <= 500 {
		summary.ScopeSize = "large"
	} else {
		summary.ScopeSize = "major"
	}

	return summary
}

func assessImpact(analysis *DiffAnalysis) *ImpactAssessment {
	impact := &ImpactAssessment{}

	for _, f := range analysis.Files {
		// Check for API changes
		if strings.Contains(f.Path, "/api/") || strings.HasPrefix(f.Path, "pkg/") {
			impact.APIChanges = true
		}

		// Check for security-relevant files
		if strings.Contains(f.Path, "auth") || strings.Contains(f.Path, "security") ||
			strings.Contains(f.Path, "crypt") || strings.Contains(f.Path, "password") {
			impact.SecurityRelevant = true
			impact.Concerns = append(impact.Concerns, fmt.Sprintf("Security-relevant change: %s", f.Path))
		}

		// Check for documentation impact
		if strings.HasSuffix(f.Path, ".md") || strings.Contains(f.Path, "/docs/") {
			impact.DocumentationImpact = true
		}
	}

	// Determine risk level
	if impact.SecurityRelevant || impact.BreakingChanges {
		impact.RiskLevel = "high"
	} else if impact.APIChanges || analysis.Summary.ScopeSize == "large" {
		impact.RiskLevel = "medium"
	} else {
		impact.RiskLevel = "low"
	}

	// Determine if testing is required
	impact.TestingRequired = impact.RiskLevel != "low" ||
		analysis.Summary.FilesChanged > 5

	// Determine if review is required
	impact.ReviewRequired = impact.RiskLevel != "low" ||
		analysis.Summary.TotalAdditions > 100

	// Generate recommendations
	if !hasTestChanges(analysis) && impact.TestingRequired {
		impact.Recommendations = append(impact.Recommendations, "Consider adding tests for these changes")
	}
	if impact.SecurityRelevant {
		impact.Recommendations = append(impact.Recommendations, "Security review recommended")
	}
	if impact.APIChanges {
		impact.Recommendations = append(impact.Recommendations, "Document API changes")
	}

	return impact
}

func generateReviewNotes(analysis *DiffAnalysis) []string {
	var notes []string

	// Group complex files
	var complexFiles []string
	for _, f := range analysis.Files {
		if f.Complexity == "complex" {
			complexFiles = append(complexFiles, f.Path)
		}
	}
	if len(complexFiles) > 0 {
		notes = append(notes, fmt.Sprintf("Complex changes: %s", strings.Join(complexFiles, ", ")))
	}

	// Note categories
	if len(analysis.Categories) > 3 {
		notes = append(notes, "Changes span multiple areas - consider splitting")
	}

	// Binary files
	for _, f := range analysis.Files {
		if f.Binary {
			notes = append(notes, fmt.Sprintf("Binary file changed: %s", f.Path))
		}
	}

	return notes
}

func formatSmartDiff(analysis *DiffAnalysis) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("## Change Summary\n\n"))
	sb.WriteString(fmt.Sprintf("**Scope:** %s (%d files, +%d/-%d lines)\n",
		analysis.Summary.ScopeSize,
		analysis.Summary.FilesChanged,
		analysis.Summary.TotalAdditions,
		analysis.Summary.TotalDeletions))

	if len(analysis.Summary.Languages) > 0 {
		sb.WriteString(fmt.Sprintf("**Languages:** %s\n", strings.Join(analysis.Summary.Languages, ", ")))
	}

	sb.WriteString(fmt.Sprintf("**Risk Level:** %s\n\n", analysis.Impact.RiskLevel))

	// Files by category
	sb.WriteString("### Changes by Category\n\n")
	for category, files := range analysis.Categories {
		sb.WriteString(fmt.Sprintf("**%s:** ", category))
		var names []string
		for _, f := range files {
			names = append(names, f.Path)
		}
		sb.WriteString(strings.Join(names, ", "))
		sb.WriteString("\n")
	}

	// Impact notes
	if len(analysis.Impact.Concerns) > 0 {
		sb.WriteString("\n### Concerns\n")
		for _, c := range analysis.Impact.Concerns {
			sb.WriteString(fmt.Sprintf("- %s\n", c))
		}
	}

	// Recommendations
	if len(analysis.Impact.Recommendations) > 0 {
		sb.WriteString("\n### Recommendations\n")
		for _, r := range analysis.Impact.Recommendations {
			sb.WriteString(fmt.Sprintf("- %s\n", r))
		}
	}

	return sb.String()
}

func identifyKeyChanges(analysis *DiffAnalysis) []string {
	var changes []string

	// Sort files by change size
	sorted := make([]FileDiff, len(analysis.Files))
	copy(sorted, analysis.Files)
	sort.Slice(sorted, func(i, j int) bool {
		return (sorted[i].Additions + sorted[i].Deletions) > (sorted[j].Additions + sorted[j].Deletions)
	})

	// Top 5 files by change size
	for i := 0; i < 5 && i < len(sorted); i++ {
		f := sorted[i]
		changes = append(changes, fmt.Sprintf("%s (+%d/-%d)", f.Path, f.Additions, f.Deletions))
	}

	return changes
}

func hasTestChanges(analysis *DiffAnalysis) bool {
	for _, f := range analysis.Files {
		if strings.Contains(f.Path, "_test.") || strings.Contains(f.Path, ".test.") ||
			strings.Contains(f.Path, "/test/") || strings.Contains(f.Path, "/tests/") {
			return true
		}
	}
	return false
}

func hasDocChanges(analysis *DiffAnalysis) bool {
	for _, f := range analysis.Files {
		if strings.HasSuffix(f.Path, ".md") || strings.Contains(f.Path, "/docs/") {
			return true
		}
	}
	return false
}

func generateChecklist(analysis *DiffAnalysis) []string {
	var checklist []string

	checklist = append(checklist, "[ ] Code follows project style guidelines")
	checklist = append(checklist, "[ ] Changes are focused and atomic")

	if !hasTestChanges(analysis) {
		checklist = append(checklist, "[ ] Tests added/updated for changes")
	} else {
		checklist = append(checklist, "[x] Tests included")
	}

	if analysis.Impact.APIChanges {
		checklist = append(checklist, "[ ] API documentation updated")
	}

	if analysis.Impact.SecurityRelevant {
		checklist = append(checklist, "[ ] Security implications reviewed")
	}

	if analysis.Summary.ScopeSize == "large" || analysis.Summary.ScopeSize == "major" {
		checklist = append(checklist, "[ ] Consider splitting into smaller PRs")
	}

	return checklist
}

func (t CommitType) String() string {
	return string(t)
}
