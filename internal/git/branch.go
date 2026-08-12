package git

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// BranchInfo contains information about a branch
type BranchInfo struct {
	Name        string
	IsRemote    bool
	IsCurrent   bool
	LastCommit  string
	LastAuthor  string
	LastDate    time.Time
	AheadCount  int
	BehindCount int
	TrackingRef string
}

// BranchContext provides context about branch relationships
type BranchContext struct {
	Current        *BranchInfo
	DefaultBranch  string
	DivergenceInfo *DivergenceInfo
	SuggestRebase  bool
	RebaseReason   string
}

// DivergenceInfo contains info about branch divergence
type DivergenceInfo struct {
	BaseBranch     string
	CommonAncestor string
	AheadCommits   int
	BehindCommits  int
	ConflictRisk   string // "low", "medium", "high"
	ConflictFiles  []string
}

// GetBranchContext analyzes the current branch context
func GetBranchContext(ctx context.Context, dir string) (*BranchContext, error) {
	bc := &BranchContext{}

	// Get current branch
	current, err := GetCurrentBranchInfo(ctx, dir)
	if err != nil {
		return nil, err
	}
	bc.Current = current

	// Determine default branch
	bc.DefaultBranch = detectDefaultBranch(ctx, dir)

	// Analyze divergence from default branch
	if bc.DefaultBranch != "" && bc.Current.Name != bc.DefaultBranch {
		bc.DivergenceInfo, _ = AnalyzeDivergence(ctx, dir, bc.Current.Name, bc.DefaultBranch)
	}

	// Determine if rebase is suggested
	bc.evaluateRebaseSuggestion()

	return bc, nil
}

// GetCurrentBranchInfo returns detailed info about current branch
func GetCurrentBranchInfo(ctx context.Context, dir string) (*BranchInfo, error) {
	name, err := CurrentBranch(ctx, dir)
	if err != nil {
		return nil, err
	}

	info := &BranchInfo{
		Name:      name,
		IsCurrent: true,
	}

	// Get last commit info
	out, err := runGit(ctx, dir, "log", "-1", "--format=%H|%an|%aI", name)
	if err == nil && out != "" {
		parts := strings.Split(strings.TrimSpace(out), "|")
		if len(parts) >= 3 {
			info.LastCommit = parts[0]
			info.LastAuthor = parts[1]
			info.LastDate, _ = time.Parse(time.RFC3339, parts[2])
		}
	}

	// Get tracking info
	tracking, _ := runGit(ctx, dir, "rev-parse", "--abbrev-ref", name+"@{upstream}")
	info.TrackingRef = strings.TrimSpace(tracking)

	// Get ahead/behind counts
	if info.TrackingRef != "" {
		ahead, behind := getAheadBehind(ctx, dir, name, info.TrackingRef)
		info.AheadCount = ahead
		info.BehindCount = behind
	}

	return info, nil
}

// ListBranches returns all branches with their info
func ListBranches(ctx context.Context, dir string, includeRemote bool) ([]*BranchInfo, error) {
	args := []string{"branch", "--format=%(refname:short)|%(objectname:short)|%(authorname)|%(authordate:iso)"}
	if includeRemote {
		args = append(args, "-a")
	}

	out, err := runGit(ctx, dir, args...)
	if err != nil {
		return nil, err
	}

	current, _ := CurrentBranch(ctx, dir)

	var branches []*BranchInfo
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) < 4 {
			continue
		}

		info := &BranchInfo{
			Name:       parts[0],
			LastCommit: parts[1],
			LastAuthor: parts[2],
			IsCurrent:  parts[0] == current,
			IsRemote:   strings.HasPrefix(parts[0], "remotes/"),
		}
		info.LastDate, _ = time.Parse("2006-01-02 15:04:05 -0700", parts[3])
		branches = append(branches, info)
	}

	return branches, nil
}

// AnalyzeDivergence analyzes divergence between two branches
func AnalyzeDivergence(ctx context.Context, dir, branch, baseBranch string) (*DivergenceInfo, error) {
	info := &DivergenceInfo{
		BaseBranch: baseBranch,
	}

	// Find merge base (common ancestor)
	ancestor, err := runGit(ctx, dir, "merge-base", branch, baseBranch)
	if err != nil {
		return nil, err
	}
	info.CommonAncestor = strings.TrimSpace(ancestor)

	// Count commits ahead and behind
	info.AheadCommits, info.BehindCommits = getAheadBehind(ctx, dir, branch, baseBranch)

	// Assess conflict risk
	info.ConflictRisk, info.ConflictFiles = assessConflictRisk(ctx, dir, branch, baseBranch)

	return info, nil
}

// SuggestRebase analyzes if rebasing would be beneficial
func SuggestRebase(ctx context.Context, dir string) (bool, string) {
	bc, err := GetBranchContext(ctx, dir)
	if err != nil {
		return false, ""
	}

	return bc.SuggestRebase, bc.RebaseReason
}

// GetBranchDiff returns the diff between current branch and base
func GetBranchDiff(ctx context.Context, dir, baseBranch string) (string, error) {
	if baseBranch == "" {
		baseBranch = detectDefaultBranch(ctx, dir)
	}

	out, err := runGit(ctx, dir, "diff", baseBranch+"...HEAD")
	if err != nil {
		return "", err
	}
	return out, nil
}

// GetBranchCommits returns commits unique to the current branch
func GetBranchCommits(ctx context.Context, dir, baseBranch string) ([]CommitInfo, error) {
	if baseBranch == "" {
		baseBranch = detectDefaultBranch(ctx, dir)
	}

	out, err := runGit(ctx, dir, "log", "--format=%H|%an|%aI|%s", baseBranch+"..HEAD")
	if err != nil {
		return nil, err
	}

	var commits []CommitInfo
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 4)
		if len(parts) < 4 {
			continue
		}
		commit := CommitInfo{
			Hash:    parts[0],
			Author:  parts[1],
			Subject: parts[3],
		}
		commit.Date, _ = time.Parse(time.RFC3339, parts[2])
		commits = append(commits, commit)
	}

	return commits, nil
}

// CommitInfo represents basic commit information
type CommitInfo struct {
	Hash    string
	Author  string
	Date    time.Time
	Subject string
}

// Helper functions

func detectDefaultBranch(ctx context.Context, dir string) string {
	// Try to get default branch from remote
	out, err := runGit(ctx, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "--short")
	if err == nil {
		ref := strings.TrimSpace(out)
		if strings.HasPrefix(ref, "origin/") {
			return strings.TrimPrefix(ref, "origin/")
		}
	}

	// Check common default branch names
	for _, name := range []string{"main", "master", "develop"} {
		if branchExists(ctx, dir, name) {
			return name
		}
	}

	return ""
}

func branchExists(ctx context.Context, dir, branch string) bool {
	_, err := runGit(ctx, dir, "rev-parse", "--verify", branch)
	return err == nil
}

func getAheadBehind(ctx context.Context, dir, branch, baseBranch string) (ahead, behind int) {
	out, err := runGit(ctx, dir, "rev-list", "--left-right", "--count", branch+"..."+baseBranch)
	if err != nil {
		return 0, 0
	}

	parts := strings.Fields(strings.TrimSpace(out))
	if len(parts) >= 2 {
		ahead, _ = strconv.Atoi(parts[0])
		behind, _ = strconv.Atoi(parts[1])
	}
	return
}

func assessConflictRisk(ctx context.Context, dir, branch, baseBranch string) (string, []string) {
	// Get files modified in current branch
	branchFiles, _ := runGit(ctx, dir, "diff", "--name-only", baseBranch+"..."+branch)
	// Get files modified in base branch since divergence
	baseFiles, _ := runGit(ctx, dir, "diff", "--name-only", branch+"..."+baseBranch)

	branchSet := make(map[string]bool)
	for _, f := range strings.Split(branchFiles, "\n") {
		if f := strings.TrimSpace(f); f != "" {
			branchSet[f] = true
		}
	}

	var conflicts []string
	for _, f := range strings.Split(baseFiles, "\n") {
		f = strings.TrimSpace(f)
		if f != "" && branchSet[f] {
			conflicts = append(conflicts, f)
		}
	}

	risk := "low"
	if len(conflicts) > 10 {
		risk = "high"
	} else if len(conflicts) > 3 {
		risk = "medium"
	} else if len(conflicts) > 0 {
		risk = "low"
	}

	return risk, conflicts
}

func (bc *BranchContext) evaluateRebaseSuggestion() {
	if bc.DivergenceInfo == nil {
		return
	}

	di := bc.DivergenceInfo

	// Suggest rebase if behind and conflict risk is low
	if di.BehindCommits > 5 && di.ConflictRisk == "low" {
		bc.SuggestRebase = true
		bc.RebaseReason = fmt.Sprintf("Branch is %d commits behind %s with low conflict risk",
			di.BehindCommits, di.BaseBranch)
		return
	}

	// Warn if significantly behind
	if di.BehindCommits > 20 {
		bc.SuggestRebase = true
		bc.RebaseReason = fmt.Sprintf("Branch is %d commits behind %s - consider syncing",
			di.BehindCommits, di.BaseBranch)
	}
}
