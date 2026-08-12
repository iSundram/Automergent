package git

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// BlameInfo contains blame information for a file
type BlameInfo struct {
	FilePath string
	Lines    []BlameLine
	Authors  map[string]*AuthorStats
	Summary  *BlameSummary
}

// BlameLine represents blame info for a single line
type BlameLine struct {
	LineNumber int
	Content    string
	CommitHash string
	Author     string
	AuthorMail string
	Timestamp  time.Time
	Summary    string
}

// AuthorStats contains statistics for an author
type AuthorStats struct {
	Name          string
	Email         string
	LinesAuthored int
	Commits       int
	FirstCommit   time.Time
	LastCommit    time.Time
	Percentage    float64
}

// BlameSummary provides an overview of file ownership
type BlameSummary struct {
	TotalLines    int
	UniqueAuthors int
	UniqueCommits int
	OldestChange  time.Time
	NewestChange  time.Time
	PrimaryAuthor string
	RecentAuthors []string
}

// ChangeHistoryEntry represents a change in the file's history
type ChangeHistoryEntry struct {
	CommitHash   string
	Author       string
	Date         time.Time
	Subject      string
	LinesChanged int
	IsMajor      bool
}

// GetBlame returns blame information for a file
func GetBlame(ctx context.Context, dir, filePath string) (*BlameInfo, error) {
	out, err := runGit(ctx, dir, "blame", "--line-porcelain", filePath)
	if err != nil {
		return nil, fmt.Errorf("git blame failed: %w", err)
	}

	info := &BlameInfo{
		FilePath: filePath,
		Authors:  make(map[string]*AuthorStats),
	}

	info.Lines = parseBlameOutput(out)

	// Build author stats
	commitSet := make(map[string]bool)
	for _, line := range info.Lines {
		author := line.Author
		if _, ok := info.Authors[author]; !ok {
			info.Authors[author] = &AuthorStats{
				Name:        author,
				Email:       line.AuthorMail,
				FirstCommit: line.Timestamp,
				LastCommit:  line.Timestamp,
			}
		}

		stats := info.Authors[author]
		stats.LinesAuthored++
		if !commitSet[line.CommitHash] {
			commitSet[line.CommitHash] = true
			stats.Commits++
		}
		if line.Timestamp.Before(stats.FirstCommit) {
			stats.FirstCommit = line.Timestamp
		}
		if line.Timestamp.After(stats.LastCommit) {
			stats.LastCommit = line.Timestamp
		}
	}

	// Calculate percentages
	totalLines := len(info.Lines)
	for _, stats := range info.Authors {
		stats.Percentage = float64(stats.LinesAuthored) / float64(totalLines) * 100
	}

	// Build summary
	info.Summary = buildBlameSummary(info)

	return info, nil
}

// GetBlameForLines returns blame info for specific lines
func GetBlameForLines(ctx context.Context, dir, filePath string, startLine, endLine int) (*BlameInfo, error) {
	out, err := runGit(ctx, dir, "blame", "--line-porcelain",
		fmt.Sprintf("-L%d,%d", startLine, endLine), filePath)
	if err != nil {
		return nil, err
	}

	info := &BlameInfo{
		FilePath: filePath,
		Lines:    parseBlameOutput(out),
		Authors:  make(map[string]*AuthorStats),
	}

	return info, nil
}

// WhoChanged identifies who made specific changes
func WhoChanged(ctx context.Context, dir, filePath, pattern string) ([]BlameLine, error) {
	blame, err := GetBlame(ctx, dir, filePath)
	if err != nil {
		return nil, err
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern: %w", err)
	}

	var matches []BlameLine
	for _, line := range blame.Lines {
		if re.MatchString(line.Content) {
			matches = append(matches, line)
		}
	}

	return matches, nil
}

// GetFileHistory returns the change history for a file
func GetFileHistory(ctx context.Context, dir, filePath string, limit int) ([]ChangeHistoryEntry, error) {
	if limit <= 0 {
		limit = 50
	}

	out, err := runGit(ctx, dir, "log", "--format=%H|%an|%aI|%s",
		"--numstat", "-n", fmt.Sprint(limit), "--", filePath)
	if err != nil {
		return nil, err
	}

	return parseFileHistory(out), nil
}

// GetRecentModifications returns recently modified files with authors
func GetRecentModifications(ctx context.Context, dir string, days int) (map[string][]string, error) {
	since := time.Now().AddDate(0, 0, -days).Format("2006-01-02")

	out, err := runGit(ctx, dir, "log", "--since="+since, "--format=%H|%an", "--name-only")
	if err != nil {
		return nil, err
	}

	// Map file -> unique authors
	result := make(map[string][]string)
	authorSets := make(map[string]map[string]bool)

	lines := strings.Split(out, "\n")
	var currentAuthor string
	for _, line := range lines {
		if strings.Contains(line, "|") {
			parts := strings.SplitN(line, "|", 2)
			if len(parts) >= 2 {
				currentAuthor = parts[1]
			}
		} else if line != "" && currentAuthor != "" {
			if _, ok := authorSets[line]; !ok {
				authorSets[line] = make(map[string]bool)
			}
			authorSets[line][currentAuthor] = true
		}
	}

	// Convert sets to slices
	for file, authors := range authorSets {
		for author := range authors {
			result[file] = append(result[file], author)
		}
	}

	return result, nil
}

// GetAuthorPatterns analyzes author contribution patterns
func GetAuthorPatterns(ctx context.Context, dir string) (map[string]*AuthorPattern, error) {
	out, err := runGit(ctx, dir, "log", "--format=%an|%aI", "--name-only", "-n", "500")
	if err != nil {
		return nil, err
	}

	patterns := make(map[string]*AuthorPattern)
	lines := strings.Split(out, "\n")

	var currentAuthor string
	var currentDate time.Time
	for _, line := range lines {
		if strings.Contains(line, "|") {
			parts := strings.SplitN(line, "|", 2)
			if len(parts) >= 2 {
				currentAuthor = parts[0]
				currentDate, _ = time.Parse(time.RFC3339, parts[1])

				if _, ok := patterns[currentAuthor]; !ok {
					patterns[currentAuthor] = &AuthorPattern{
						Name:       currentAuthor,
						FileAreas:  make(map[string]int),
						CommitDays: make(map[string]int),
					}
				}

				// Track commit days
				dayKey := currentDate.Format("Monday")
				patterns[currentAuthor].CommitDays[dayKey]++
				patterns[currentAuthor].TotalCommits++
			}
		} else if line != "" && currentAuthor != "" {
			p := patterns[currentAuthor]
			// Track file areas
			parts := strings.Split(line, "/")
			if len(parts) > 1 {
				p.FileAreas[parts[0]]++
			}
		}
	}

	// Analyze patterns
	for _, p := range patterns {
		p.analyze()
	}

	return patterns, nil
}

// AuthorPattern represents contribution patterns for an author
type AuthorPattern struct {
	Name         string
	TotalCommits int
	FileAreas    map[string]int
	CommitDays   map[string]int
	PrimaryAreas []string
	ActiveDays   []string
	ContribStyle string // "focused", "broad", "sporadic"
}

func (p *AuthorPattern) analyze() {
	// Find primary areas
	type kv struct {
		Key   string
		Value int
	}
	var areas []kv
	for k, v := range p.FileAreas {
		areas = append(areas, kv{k, v})
	}
	sort.Slice(areas, func(i, j int) bool {
		return areas[i].Value > areas[j].Value
	})

	// Top 3 areas
	for i := 0; i < 3 && i < len(areas); i++ {
		p.PrimaryAreas = append(p.PrimaryAreas, areas[i].Key)
	}

	// Find most active days
	var days []kv
	for k, v := range p.CommitDays {
		days = append(days, kv{k, v})
	}
	sort.Slice(days, func(i, j int) bool {
		return days[i].Value > days[j].Value
	})
	for i := 0; i < 2 && i < len(days); i++ {
		p.ActiveDays = append(p.ActiveDays, days[i].Key)
	}

	// Determine contribution style
	if len(p.PrimaryAreas) == 1 && p.FileAreas[p.PrimaryAreas[0]] > p.TotalCommits/2 {
		p.ContribStyle = "focused"
	} else if len(p.FileAreas) > 5 {
		p.ContribStyle = "broad"
	} else {
		p.ContribStyle = "balanced"
	}
}

// GetCodeOwners identifies likely code owners for a file
func GetCodeOwners(ctx context.Context, dir, filePath string) ([]string, error) {
	blame, err := GetBlame(ctx, dir, filePath)
	if err != nil {
		return nil, err
	}

	// Sort authors by contribution
	type authorContrib struct {
		name       string
		percentage float64
		recent     bool
	}

	var contribs []authorContrib
	oneMonthAgo := time.Now().AddDate(0, -1, 0)

	for _, stats := range blame.Authors {
		contribs = append(contribs, authorContrib{
			name:       stats.Name,
			percentage: stats.Percentage,
			recent:     stats.LastCommit.After(oneMonthAgo),
		})
	}

	// Sort by percentage, with boost for recent contributors
	sort.Slice(contribs, func(i, j int) bool {
		scoreI := contribs[i].percentage
		scoreJ := contribs[j].percentage
		if contribs[i].recent {
			scoreI *= 1.5
		}
		if contribs[j].recent {
			scoreJ *= 1.5
		}
		return scoreI > scoreJ
	})

	// Return top contributors
	var owners []string
	for i := 0; i < 3 && i < len(contribs); i++ {
		owners = append(owners, contribs[i].name)
	}

	return owners, nil
}

// Helper functions

func parseBlameOutput(output string) []BlameLine {
	var lines []BlameLine
	var current BlameLine
	lineNum := 0

	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "\t") {
			// Content line
			current.Content = line[1:]
			current.LineNumber = lineNum
			lines = append(lines, current)
			current = BlameLine{}
		} else if len(line) > 40 && line[40] == ' ' {
			// Commit hash line
			lineNum++
			current.CommitHash = line[:40]
		} else if strings.HasPrefix(line, "author ") {
			current.Author = strings.TrimPrefix(line, "author ")
		} else if strings.HasPrefix(line, "author-mail ") {
			current.AuthorMail = strings.Trim(strings.TrimPrefix(line, "author-mail "), "<>")
		} else if strings.HasPrefix(line, "author-time ") {
			timestamp := strings.TrimPrefix(line, "author-time ")
			var ts int64
			fmt.Sscanf(timestamp, "%d", &ts)
			current.Timestamp = time.Unix(ts, 0)
		} else if strings.HasPrefix(line, "summary ") {
			current.Summary = strings.TrimPrefix(line, "summary ")
		}
	}

	return lines
}

func buildBlameSummary(info *BlameInfo) *BlameSummary {
	summary := &BlameSummary{
		TotalLines:    len(info.Lines),
		UniqueAuthors: len(info.Authors),
	}

	commitSet := make(map[string]bool)
	var primaryAuthor string
	var maxLines int

	for _, line := range info.Lines {
		commitSet[line.CommitHash] = true

		if summary.OldestChange.IsZero() || line.Timestamp.Before(summary.OldestChange) {
			summary.OldestChange = line.Timestamp
		}
		if line.Timestamp.After(summary.NewestChange) {
			summary.NewestChange = line.Timestamp
		}
	}

	summary.UniqueCommits = len(commitSet)

	// Find primary author and recent authors
	oneMonthAgo := time.Now().AddDate(0, -1, 0)
	recentSet := make(map[string]bool)

	for author, stats := range info.Authors {
		if stats.LinesAuthored > maxLines {
			maxLines = stats.LinesAuthored
			primaryAuthor = author
		}
		if stats.LastCommit.After(oneMonthAgo) {
			recentSet[author] = true
		}
	}

	summary.PrimaryAuthor = primaryAuthor
	for author := range recentSet {
		summary.RecentAuthors = append(summary.RecentAuthors, author)
	}

	return summary
}

func parseFileHistory(output string) []ChangeHistoryEntry {
	var entries []ChangeHistoryEntry
	var current ChangeHistoryEntry

	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "|") {
			// Save previous entry
			if current.CommitHash != "" {
				current.IsMajor = current.LinesChanged > 50
				entries = append(entries, current)
			}

			parts := strings.SplitN(line, "|", 4)
			if len(parts) >= 4 {
				current = ChangeHistoryEntry{
					CommitHash: parts[0],
					Author:     parts[1],
					Subject:    parts[3],
				}
				current.Date, _ = time.Parse(time.RFC3339, parts[2])
			}
		} else if line != "" && current.CommitHash != "" {
			// Numstat line: additions \t deletions \t filename
			var add, del int
			fmt.Sscanf(line, "%d\t%d", &add, &del)
			current.LinesChanged += add + del
		}
	}

	// Add last entry
	if current.CommitHash != "" {
		current.IsMajor = current.LinesChanged > 50
		entries = append(entries, current)
	}

	return entries
}
