package git

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// ConflictType represents the type of merge conflict
type ConflictType string

const (
	ConflictTypeBothModified ConflictType = "both_modified"
	ConflictTypeDeleteModify ConflictType = "delete_modify"
	ConflictTypeModifyDelete ConflictType = "modify_delete"
	ConflictTypeAddAdd       ConflictType = "add_add"
	ConflictTypeRenameRename ConflictType = "rename_rename"
	ConflictTypeRenamedelete ConflictType = "rename_delete"
)

// ConflictInfo represents a potential or actual merge conflict
type ConflictInfo struct {
	FilePath       string
	Type           ConflictType
	OurChanges     string
	TheirChanges   string
	CommonAncestor string
	CanAutoResolve bool
	Resolution     *ResolutionSuggestion
	Severity       string // "low", "medium", "high"
}

// ResolutionSuggestion provides guidance for resolving conflicts
type ResolutionSuggestion struct {
	Strategy    string // "ours", "theirs", "union", "manual"
	Explanation string
	AutoResolve string // The automatically resolved content
	Confidence  float64
}

// MergePreview represents a preview of a merge operation
type MergePreview struct {
	SourceBranch  string
	TargetBranch  string
	HasConflicts  bool
	Conflicts     []ConflictInfo
	SafeToMerge   bool
	FilesChanged  int
	Additions     int
	Deletions     int
	AffectedAreas []string
}

// PreviewMerge performs a dry-run merge and detects conflicts
func PreviewMerge(ctx context.Context, dir, sourceBranch, targetBranch string) (*MergePreview, error) {
	preview := &MergePreview{
		SourceBranch: sourceBranch,
		TargetBranch: targetBranch,
	}

	if targetBranch == "" {
		current, err := CurrentBranch(ctx, dir)
		if err != nil {
			return nil, err
		}
		preview.TargetBranch = current
	}

	// Get diff stats
	stats, _ := runGit(ctx, dir, "diff", "--stat", preview.TargetBranch+"..."+preview.SourceBranch)
	preview.FilesChanged, preview.Additions, preview.Deletions = parseDiffStats(stats)

	// Check for potential conflicts using merge-tree
	conflicts, err := detectMergeConflicts(ctx, dir, preview.SourceBranch, preview.TargetBranch)
	if err != nil {
		// Fallback to file comparison
		conflicts = detectConflictsByComparison(ctx, dir, preview.SourceBranch, preview.TargetBranch)
	}

	preview.Conflicts = conflicts
	preview.HasConflicts = len(conflicts) > 0
	preview.SafeToMerge = !preview.HasConflicts

	// Identify affected areas
	preview.AffectedAreas = getAffectedAreas(ctx, dir, preview.SourceBranch, preview.TargetBranch)

	return preview, nil
}

// DetectCurrentConflicts detects conflicts in the current merge state
func DetectCurrentConflicts(ctx context.Context, dir string) ([]ConflictInfo, error) {
	// Check if we're in a merge state
	status, err := runGit(ctx, dir, "status", "--porcelain")
	if err != nil {
		return nil, err
	}

	var conflicts []ConflictInfo
	for _, line := range strings.Split(status, "\n") {
		if len(line) < 3 {
			continue
		}

		// Conflict markers: UU, AA, DD, AU, UA, DU, UD
		xy := line[:2]
		if isConflictStatus(xy) {
			path := strings.TrimSpace(line[3:])
			conflict := ConflictInfo{
				FilePath: path,
				Type:     classifyConflictFromStatus(xy),
			}

			// Get conflict content
			conflict.OurChanges, conflict.TheirChanges, conflict.CommonAncestor =
				extractConflictContent(ctx, dir, path)

			// Analyze and suggest resolution
			conflict.Resolution = suggestResolution(&conflict)
			conflict.CanAutoResolve = conflict.Resolution != nil && conflict.Resolution.Confidence > 0.8
			conflict.Severity = assessConflictSeverity(&conflict)

			conflicts = append(conflicts, conflict)
		}
	}

	return conflicts, nil
}

// ResolveConflict attempts to resolve a conflict
func ResolveConflict(ctx context.Context, dir string, conflict *ConflictInfo, strategy string) error {
	switch strategy {
	case "ours":
		_, err := runGit(ctx, dir, "checkout", "--ours", conflict.FilePath)
		if err != nil {
			return err
		}
	case "theirs":
		_, err := runGit(ctx, dir, "checkout", "--theirs", conflict.FilePath)
		if err != nil {
			return err
		}
	case "union":
		// For simple text conflicts, try union merge
		return resolveUnion(ctx, dir, conflict)
	default:
		return fmt.Errorf("unknown strategy: %s", strategy)
	}

	// Stage the resolved file
	_, err := runGit(ctx, dir, "add", conflict.FilePath)
	return err
}

// AutoResolveConflicts attempts to automatically resolve safe conflicts
func AutoResolveConflicts(ctx context.Context, dir string) (resolved, failed []ConflictInfo, err error) {
	conflicts, err := DetectCurrentConflicts(ctx, dir)
	if err != nil {
		return nil, nil, err
	}

	for _, c := range conflicts {
		if c.CanAutoResolve && c.Resolution != nil {
			err := ResolveConflict(ctx, dir, &c, c.Resolution.Strategy)
			if err != nil {
				failed = append(failed, c)
			} else {
				resolved = append(resolved, c)
			}
		} else {
			failed = append(failed, c)
		}
	}

	return resolved, failed, nil
}

// ExplainConflict provides a human-readable explanation of a conflict
func ExplainConflict(conflict *ConflictInfo) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("## Conflict in %s\n\n", conflict.FilePath))
	sb.WriteString(fmt.Sprintf("**Type:** %s\n", conflictTypeDescription(conflict.Type)))
	sb.WriteString(fmt.Sprintf("**Severity:** %s\n\n", conflict.Severity))

	sb.WriteString("### What happened:\n")
	switch conflict.Type {
	case ConflictTypeBothModified:
		sb.WriteString("Both branches modified the same part of this file.\n")
	case ConflictTypeDeleteModify:
		sb.WriteString("You deleted this file, but the other branch modified it.\n")
	case ConflictTypeModifyDelete:
		sb.WriteString("You modified this file, but the other branch deleted it.\n")
	case ConflictTypeAddAdd:
		sb.WriteString("Both branches added a file with the same name.\n")
	}

	if conflict.Resolution != nil {
		sb.WriteString(fmt.Sprintf("\n### Suggested resolution:\n%s\n", conflict.Resolution.Explanation))
		sb.WriteString(fmt.Sprintf("\n**Strategy:** %s (confidence: %.0f%%)\n",
			conflict.Resolution.Strategy, conflict.Resolution.Confidence*100))
	}

	return sb.String()
}

// Helper functions

func detectMergeConflicts(ctx context.Context, dir, source, target string) ([]ConflictInfo, error) {
	// Use git merge-tree for conflict detection (Git 2.38+)
	ancestor, err := runGit(ctx, dir, "merge-base", source, target)
	if err != nil {
		return nil, err
	}
	ancestor = strings.TrimSpace(ancestor)

	out, err := runGit(ctx, dir, "merge-tree", ancestor, target, source)
	if err != nil {
		return nil, err
	}

	return parseMergeTreeOutput(out), nil
}

func detectConflictsByComparison(ctx context.Context, dir, source, target string) []ConflictInfo {
	// Get files changed in each branch since common ancestor
	ancestor, _ := runGit(ctx, dir, "merge-base", source, target)
	ancestor = strings.TrimSpace(ancestor)

	sourceFiles := getChangedFiles(ctx, dir, ancestor, source)
	targetFiles := getChangedFiles(ctx, dir, ancestor, target)

	// Find overlapping files
	var conflicts []ConflictInfo
	for file, sourceStatus := range sourceFiles {
		if targetStatus, ok := targetFiles[file]; ok {
			conflict := ConflictInfo{
				FilePath: file,
				Type:     classifyConflictType(sourceStatus, targetStatus),
			}
			conflict.Severity = assessConflictSeverity(&conflict)
			conflicts = append(conflicts, conflict)
		}
	}

	return conflicts
}

func getChangedFiles(ctx context.Context, dir, from, to string) map[string]string {
	out, _ := runGit(ctx, dir, "diff", "--name-status", from+".."+to)
	files := make(map[string]string)

	for _, line := range strings.Split(out, "\n") {
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			files[parts[1]] = parts[0]
		}
	}

	return files
}

func isConflictStatus(xy string) bool {
	conflictMarkers := []string{"UU", "AA", "DD", "AU", "UA", "DU", "UD"}
	for _, m := range conflictMarkers {
		if xy == m {
			return true
		}
	}
	return false
}

func classifyConflictFromStatus(xy string) ConflictType {
	switch xy {
	case "UU":
		return ConflictTypeBothModified
	case "AA":
		return ConflictTypeAddAdd
	case "DD":
		return ConflictTypeDeleteModify
	case "AU", "UA":
		return ConflictTypeModifyDelete
	case "DU":
		return ConflictTypeDeleteModify
	case "UD":
		return ConflictTypeModifyDelete
	}
	return ConflictTypeBothModified
}

func classifyConflictType(sourceStatus, targetStatus string) ConflictType {
	if sourceStatus == "M" && targetStatus == "M" {
		return ConflictTypeBothModified
	}
	if sourceStatus == "A" && targetStatus == "A" {
		return ConflictTypeAddAdd
	}
	if sourceStatus == "D" && targetStatus == "M" {
		return ConflictTypeDeleteModify
	}
	if sourceStatus == "M" && targetStatus == "D" {
		return ConflictTypeModifyDelete
	}
	return ConflictTypeBothModified
}

func extractConflictContent(ctx context.Context, dir, path string) (ours, theirs, ancestor string) {
	ours, _ = runGit(ctx, dir, "show", ":2:"+path)
	theirs, _ = runGit(ctx, dir, "show", ":3:"+path)
	ancestor, _ = runGit(ctx, dir, "show", ":1:"+path)
	return
}

func suggestResolution(conflict *ConflictInfo) *ResolutionSuggestion {
	suggestion := &ResolutionSuggestion{}

	switch conflict.Type {
	case ConflictTypeBothModified:
		// Analyze the changes to suggest a strategy
		if conflict.OurChanges == "" {
			suggestion.Strategy = "theirs"
			suggestion.Explanation = "Your version is empty; use theirs"
			suggestion.Confidence = 0.9
		} else if conflict.TheirChanges == "" {
			suggestion.Strategy = "ours"
			suggestion.Explanation = "Their version is empty; keep yours"
			suggestion.Confidence = 0.9
		} else if areChangesNonOverlapping(conflict.OurChanges, conflict.TheirChanges) {
			suggestion.Strategy = "union"
			suggestion.Explanation = "Changes are in different sections; can be combined"
			suggestion.Confidence = 0.7
		} else {
			suggestion.Strategy = "manual"
			suggestion.Explanation = "Changes overlap; manual review required"
			suggestion.Confidence = 0.3
		}

	case ConflictTypeDeleteModify:
		suggestion.Strategy = "theirs"
		suggestion.Explanation = "Consider keeping the modified version over deletion"
		suggestion.Confidence = 0.6

	case ConflictTypeModifyDelete:
		suggestion.Strategy = "ours"
		suggestion.Explanation = "You modified this file; consider keeping your changes"
		suggestion.Confidence = 0.6

	case ConflictTypeAddAdd:
		suggestion.Strategy = "manual"
		suggestion.Explanation = "Both branches added this file; review both versions"
		suggestion.Confidence = 0.2
	}

	return suggestion
}

func areChangesNonOverlapping(ours, theirs string) bool {
	// Simple heuristic: check if changes are in different line ranges
	// This is a simplified check; real implementation would use diff analysis
	ourLines := strings.Split(ours, "\n")
	theirLines := strings.Split(theirs, "\n")

	// If lengths are very different, changes likely affect different areas
	diff := len(ourLines) - len(theirLines)
	if diff < 0 {
		diff = -diff
	}

	return diff > len(ourLines)/2
}

func assessConflictSeverity(conflict *ConflictInfo) string {
	switch conflict.Type {
	case ConflictTypeAddAdd:
		return "high"
	case ConflictTypeBothModified:
		if conflict.Resolution != nil && conflict.Resolution.Confidence < 0.5 {
			return "high"
		}
		return "medium"
	case ConflictTypeDeleteModify, ConflictTypeModifyDelete:
		return "medium"
	default:
		return "low"
	}
}

func resolveUnion(ctx context.Context, dir string, conflict *ConflictInfo) error {
	// Simple union merge for text files
	// This joins both versions, removing duplicate lines
	oursLines := strings.Split(conflict.OurChanges, "\n")
	theirsLines := strings.Split(conflict.TheirChanges, "\n")

	seen := make(map[string]bool)
	var merged []string

	for _, line := range oursLines {
		if !seen[line] {
			merged = append(merged, line)
			seen[line] = true
		}
	}

	for _, line := range theirsLines {
		if !seen[line] {
			merged = append(merged, line)
			seen[line] = true
		}
	}

	// Write merged content
	content := strings.Join(merged, "\n")
	return writeFile(dir, conflict.FilePath, content)
}

func conflictTypeDescription(t ConflictType) string {
	switch t {
	case ConflictTypeBothModified:
		return "Both Modified"
	case ConflictTypeDeleteModify:
		return "Delete/Modify"
	case ConflictTypeModifyDelete:
		return "Modify/Delete"
	case ConflictTypeAddAdd:
		return "Both Added"
	case ConflictTypeRenameRename:
		return "Rename Conflict"
	case ConflictTypeRenamedelete:
		return "Rename/Delete"
	default:
		return "Unknown"
	}
}

func parseMergeTreeOutput(output string) []ConflictInfo {
	var conflicts []ConflictInfo

	// Parse merge-tree output for conflicts
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "+<<<<<<") || strings.Contains(line, "CONFLICT") {
			// Extract file path from conflict marker
			re := regexp.MustCompile(`CONFLICT \([^)]+\): (.+)`)
			matches := re.FindStringSubmatch(line)
			if len(matches) > 1 {
				conflicts = append(conflicts, ConflictInfo{
					FilePath: matches[1],
					Type:     ConflictTypeBothModified,
				})
			}
		}
	}

	return conflicts
}

func parseDiffStats(stats string) (files, additions, deletions int) {
	lines := strings.Split(stats, "\n")
	for _, line := range lines {
		// Summary line: "N files changed, N insertions(+), N deletions(-)"
		if strings.Contains(line, "files changed") || strings.Contains(line, "file changed") {
			fmt.Sscanf(line, "%d file", &files)
			if idx := strings.Index(line, "insertion"); idx > 0 {
				fmt.Sscanf(line[idx-10:idx], "%d", &additions)
			}
			if idx := strings.Index(line, "deletion"); idx > 0 {
				fmt.Sscanf(line[idx-10:idx], "%d", &deletions)
			}
		}
	}
	return
}

func getAffectedAreas(ctx context.Context, dir, source, target string) []string {
	out, _ := runGit(ctx, dir, "diff", "--name-only", target+"..."+source)
	areas := make(map[string]bool)

	for _, file := range strings.Split(out, "\n") {
		if file == "" {
			continue
		}
		parts := strings.Split(file, "/")
		if len(parts) > 1 {
			areas[parts[0]] = true
		}
	}

	var result []string
	for area := range areas {
		result = append(result, area)
	}
	return result
}
