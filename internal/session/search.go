package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// SearchIndex indexes sessions for fast searching.
type SearchIndex struct {
	dir     string
	mu      sync.RWMutex
	entries map[string]*IndexEntry
	tags    map[string][]string // tag -> session IDs
}

// IndexEntry represents an indexed session.
type IndexEntry struct {
	SessionID    string            `json:"session_id"`
	Title        string            `json:"title"`
	Summary      string            `json:"summary,omitempty"`
	Keywords     []string          `json:"keywords,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
	ProjectPath  string            `json:"project_path,omitempty"`
	WorkDir      string            `json:"work_dir,omitempty"`
	FilesTouched []string          `json:"files_touched,omitempty"`
	ToolsUsed    []string          `json:"tools_used,omitempty"`
	TaskType     string            `json:"task_type,omitempty"` // e.g., "debug", "feature", "refactor"
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	MessageCount int               `json:"message_count"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// SearchResult represents a search match.
type SearchResult struct {
	Entry      *IndexEntry `json:"entry"`
	Score      float64     `json:"score"`
	Highlights []string    `json:"highlights,omitempty"`
}

// SearchQuery represents a search query.
type SearchQuery struct {
	Text        string
	Tags        []string
	ProjectPath string
	WorkDir     string
	TaskType    string
	TimeRange   *TimeRange
	Limit       int
}

// TimeRange specifies a time window for searches.
type TimeRange struct {
	After  time.Time
	Before time.Time
}

const (
	indexFileName = "search_index.json"
	maxKeywords   = 50
	defaultLimit  = 20
)

// NewSearchIndex creates a new search index.
func NewSearchIndex(dir string) (*SearchIndex, error) {
	indexDir := filepath.Join(dir, "index")
	if err := os.MkdirAll(indexDir, 0o700); err != nil {
		return nil, fmt.Errorf("search index: mkdir: %w", err)
	}
	si := &SearchIndex{
		dir:     indexDir,
		entries: make(map[string]*IndexEntry),
		tags:    make(map[string][]string),
	}
	_ = si.load()
	return si, nil
}

// load reads the index from disk.
func (si *SearchIndex) load() error {
	indexPath := filepath.Join(si.dir, indexFileName)
	data, err := os.ReadFile(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var entries map[string]*IndexEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return err
	}

	si.entries = entries
	si.rebuildTagIndex()
	return nil
}

// save persists the index to disk.
func (si *SearchIndex) save() error {
	data, err := json.MarshalIndent(si.entries, "", "  ")
	if err != nil {
		return err
	}
	indexPath := filepath.Join(si.dir, indexFileName)
	return atomicWriteFile(indexPath, data, 0o600)
}

// rebuildTagIndex rebuilds the tag lookup map.
func (si *SearchIndex) rebuildTagIndex() {
	si.tags = make(map[string][]string)
	for id, entry := range si.entries {
		for _, tag := range entry.Tags {
			si.tags[tag] = append(si.tags[tag], id)
		}
	}
}

// IndexSession indexes a session for searching.
func (si *SearchIndex) IndexSession(sess *Session, state *PersistenceState) error {
	si.mu.Lock()
	defer si.mu.Unlock()

	entry := &IndexEntry{
		SessionID:    sess.ID,
		Title:        sess.Title,
		CreatedAt:    sess.CreatedAt,
		UpdatedAt:    sess.UpdatedAt,
		MessageCount: len(sess.Messages),
		Metadata:     sess.Metadata,
	}

	if state != nil {
		entry.ProjectPath = state.ProjectPath
		entry.WorkDir = state.WorkDir
	}

	// Extract keywords from messages
	entry.Keywords = si.extractKeywords(sess)

	// Extract tools used and files touched
	entry.ToolsUsed, entry.FilesTouched = si.extractToolsAndFiles(sess)

	// Infer task type
	entry.TaskType = si.inferTaskType(sess)

	// Generate summary from first user message
	entry.Summary = si.generateSummary(sess)

	si.entries[sess.ID] = entry
	si.rebuildTagIndex()

	return si.save()
}

// extractKeywords extracts searchable keywords from session messages.
func (si *SearchIndex) extractKeywords(sess *Session) []string {
	keywordSet := make(map[string]int)

	for _, msg := range sess.Messages {
		text := msg.TextContent()
		words := tokenize(text)
		for _, word := range words {
			if len(word) >= 3 && !isStopWord(word) {
				keywordSet[strings.ToLower(word)]++
			}
		}
	}

	// Sort by frequency and take top keywords
	type kv struct {
		k string
		v int
	}
	var sorted []kv
	for k, v := range keywordSet {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].v > sorted[j].v
	})

	keywords := make([]string, 0, maxKeywords)
	for i, item := range sorted {
		if i >= maxKeywords {
			break
		}
		keywords = append(keywords, item.k)
	}
	return keywords
}

// extractToolsAndFiles extracts tool usage and file paths from session.
func (si *SearchIndex) extractToolsAndFiles(sess *Session) (tools []string, files []string) {
	toolSet := make(map[string]bool)
	fileSet := make(map[string]bool)

	for _, msg := range sess.Messages {
		for _, call := range msg.ToolCallParts() {
			toolSet[call.Name] = true

			// Extract file paths from common tool arguments
			if path, ok := call.Args["path"].(string); ok {
				fileSet[path] = true
			}
			if file, ok := call.Args["file"].(string); ok {
				fileSet[file] = true
			}
		}
	}

	for t := range toolSet {
		tools = append(tools, t)
	}
	for f := range fileSet {
		files = append(files, f)
	}
	sort.Strings(tools)
	sort.Strings(files)
	return
}

// inferTaskType infers the type of task from session content.
func (si *SearchIndex) inferTaskType(sess *Session) string {
	if len(sess.Messages) == 0 {
		return ""
	}

	text := strings.ToLower(sess.Messages[0].TextContent())

	taskIndicators := map[string][]string{
		"debug":    {"fix", "bug", "error", "issue", "broken", "crash", "failing"},
		"feature":  {"add", "create", "implement", "new", "feature", "build"},
		"refactor": {"refactor", "clean", "improve", "optimize", "restructure"},
		"docs":     {"document", "docs", "readme", "comment", "explain"},
		"test":     {"test", "testing", "spec", "coverage"},
		"config":   {"configure", "config", "setup", "install"},
		"review":   {"review", "check", "audit", "analyze"},
	}

	bestType := ""
	bestScore := 0
	for taskType, indicators := range taskIndicators {
		score := 0
		for _, indicator := range indicators {
			if strings.Contains(text, indicator) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestType = taskType
		}
	}

	return bestType
}

// generateSummary creates a brief summary from the first user message.
func (si *SearchIndex) generateSummary(sess *Session) string {
	for _, msg := range sess.Messages {
		if msg.Role == "user" {
			text := msg.TextContent()
			if len(text) > 200 {
				text = text[:200] + "..."
			}
			return strings.TrimSpace(text)
		}
	}
	return ""
}

// Search finds sessions matching the query.
func (si *SearchIndex) Search(query SearchQuery) []SearchResult {
	si.mu.RLock()
	defer si.mu.RUnlock()

	var results []SearchResult

	for _, entry := range si.entries {
		score := si.scoreEntry(entry, query)
		if score > 0 {
			results = append(results, SearchResult{
				Entry:      entry,
				Score:      score,
				Highlights: si.findHighlights(entry, query.Text),
			})
		}
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	limit := query.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	if len(results) > limit {
		results = results[:limit]
	}

	return results
}

// scoreEntry calculates relevance score for an entry against a query.
func (si *SearchIndex) scoreEntry(entry *IndexEntry, query SearchQuery) float64 {
	score := 0.0

	// Text search
	if query.Text != "" {
		textLower := strings.ToLower(query.Text)
		queryTerms := tokenize(textLower)

		// Dedupe query terms
		termSet := make(map[string]bool)
		uniqueTerms := []string{}
		for _, term := range queryTerms {
			if !termSet[term] {
				termSet[term] = true
				uniqueTerms = append(uniqueTerms, term)
			}
		}

		// Build lowercased joined blobs
		titleLower := strings.ToLower(entry.Title)
		summaryLower := strings.ToLower(entry.Summary)

		keywordBlob := strings.Join(entry.Keywords, " ")
		keywordBlobLower := strings.ToLower(keywordBlob)

		fileBlob := strings.Join(entry.FilesTouched, " ")
		fileBlobLower := strings.ToLower(fileBlob)

		// Check each unique query term once
		for _, term := range uniqueTerms {
			// Title match (highest weight)
			if strings.Contains(titleLower, term) {
				score += 10.0
			}

			// Summary match
			if strings.Contains(summaryLower, term) {
				score += 5.0
			}

			// Keyword match
			if strings.Contains(keywordBlobLower, term) {
				score += 2.0
			}

			// File path match
			if strings.Contains(fileBlobLower, term) {
				score += 3.0
			}
		}
	}

	// Tag filter (required match)
	if len(query.Tags) > 0 {
		tagSet := make(map[string]bool)
		for _, t := range entry.Tags {
			tagSet[t] = true
		}
		for _, qt := range query.Tags {
			if !tagSet[qt] {
				return 0 // Tag required but not present
			}
			score += 5.0
		}
	}

	// Project path filter
	if query.ProjectPath != "" {
		if entry.ProjectPath != query.ProjectPath {
			return 0
		}
		score += 10.0
	}

	// Work dir filter
	if query.WorkDir != "" {
		if !strings.HasPrefix(entry.WorkDir, query.WorkDir) {
			return 0
		}
		score += 5.0
	}

	// Task type filter
	if query.TaskType != "" {
		if entry.TaskType != query.TaskType {
			return 0
		}
		score += 5.0
	}

	// Time range filter
	if query.TimeRange != nil {
		if !query.TimeRange.After.IsZero() && entry.CreatedAt.Before(query.TimeRange.After) {
			return 0
		}
		if !query.TimeRange.Before.IsZero() && entry.CreatedAt.After(query.TimeRange.Before) {
			return 0
		}
	}

	// Recency boost (more recent = higher score)
	age := time.Since(entry.UpdatedAt)
	if age < 24*time.Hour {
		score *= 1.5
	} else if age < 7*24*time.Hour {
		score *= 1.2
	}

	return score
}

// findHighlights extracts matching snippets for highlighting.
func (si *SearchIndex) findHighlights(entry *IndexEntry, query string) []string {
	if query == "" {
		return nil
	}

	var highlights []string
	queryLower := strings.ToLower(query)

	// Check title
	if strings.Contains(strings.ToLower(entry.Title), queryLower) {
		highlights = append(highlights, "Title: "+entry.Title)
	}

	// Check summary
	if strings.Contains(strings.ToLower(entry.Summary), queryLower) {
		highlights = append(highlights, "Summary: "+entry.Summary)
	}

	// Check files
	for _, file := range entry.FilesTouched {
		if strings.Contains(strings.ToLower(file), queryLower) {
			highlights = append(highlights, "File: "+file)
			if len(highlights) >= 5 {
				break
			}
		}
	}

	return highlights
}

// FindSimilar finds sessions similar to a given session.
func (si *SearchIndex) FindSimilar(sessionID string, limit int) []SearchResult {
	si.mu.RLock()
	defer si.mu.RUnlock()

	source, ok := si.entries[sessionID]
	if !ok {
		return nil
	}

	var results []SearchResult

	for id, entry := range si.entries {
		if id == sessionID {
			continue
		}

		score := si.similarityScore(source, entry)
		if score > 0 {
			results = append(results, SearchResult{
				Entry: entry,
				Score: score,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results
}

// similarityScore calculates similarity between two index entries.
func (si *SearchIndex) similarityScore(a, b *IndexEntry) float64 {
	score := 0.0

	// Same project
	if a.ProjectPath != "" && a.ProjectPath == b.ProjectPath {
		score += 20.0
	}

	// Same task type
	if a.TaskType != "" && a.TaskType == b.TaskType {
		score += 10.0
	}

	// Overlapping keywords (Jaccard-like)
	aKeywords := make(map[string]bool)
	for _, k := range a.Keywords {
		aKeywords[k] = true
	}
	overlap := 0
	for _, k := range b.Keywords {
		if aKeywords[k] {
			overlap++
		}
	}
	if len(a.Keywords)+len(b.Keywords) > 0 {
		jaccardish := float64(overlap) / float64(len(a.Keywords)+len(b.Keywords)-overlap)
		score += jaccardish * 30.0
	}

	// Overlapping files
	aFiles := make(map[string]bool)
	for _, f := range a.FilesTouched {
		aFiles[f] = true
	}
	fileOverlap := 0
	for _, f := range b.FilesTouched {
		if aFiles[f] {
			fileOverlap++
		}
	}
	if len(a.FilesTouched)+len(b.FilesTouched) > 0 {
		fileJaccard := float64(fileOverlap) / float64(len(a.FilesTouched)+len(b.FilesTouched)-fileOverlap)
		score += fileJaccard * 25.0
	}

	// Overlapping tools
	aTools := make(map[string]bool)
	for _, t := range a.ToolsUsed {
		aTools[t] = true
	}
	toolOverlap := 0
	for _, t := range b.ToolsUsed {
		if aTools[t] {
			toolOverlap++
		}
	}
	if len(a.ToolsUsed)+len(b.ToolsUsed) > 0 {
		toolJaccard := float64(toolOverlap) / float64(len(a.ToolsUsed)+len(b.ToolsUsed)-toolOverlap)
		score += toolJaccard * 15.0
	}

	return score
}

// AddTag adds a tag to a session.
func (si *SearchIndex) AddTag(sessionID, tag string) error {
	si.mu.Lock()
	defer si.mu.Unlock()

	entry, ok := si.entries[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	// Check if tag already exists
	for _, t := range entry.Tags {
		if t == tag {
			return nil
		}
	}

	entry.Tags = append(entry.Tags, tag)
	si.tags[tag] = append(si.tags[tag], sessionID)
	return si.save()
}

// RemoveTag removes a tag from a session.
func (si *SearchIndex) RemoveTag(sessionID, tag string) error {
	si.mu.Lock()
	defer si.mu.Unlock()

	entry, ok := si.entries[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	newTags := make([]string, 0, len(entry.Tags))
	for _, t := range entry.Tags {
		if t != tag {
			newTags = append(newTags, t)
		}
	}
	entry.Tags = newTags
	si.rebuildTagIndex()
	return si.save()
}

// GetByTag returns all sessions with a given tag.
func (si *SearchIndex) GetByTag(tag string) []*IndexEntry {
	si.mu.RLock()
	defer si.mu.RUnlock()

	ids := si.tags[tag]
	results := make([]*IndexEntry, 0, len(ids))
	for _, id := range ids {
		if entry, ok := si.entries[id]; ok {
			results = append(results, entry)
		}
	}
	return results
}

// GetEntry retrieves an index entry by session ID.
func (si *SearchIndex) GetEntry(sessionID string) (*IndexEntry, bool) {
	si.mu.RLock()
	defer si.mu.RUnlock()
	entry, ok := si.entries[sessionID]
	return entry, ok
}

// Delete removes a session from the index.
func (si *SearchIndex) Delete(sessionID string) error {
	si.mu.Lock()
	defer si.mu.Unlock()

	delete(si.entries, sessionID)
	si.rebuildTagIndex()
	return si.save()
}

// Stats returns statistics about the index.
func (si *SearchIndex) Stats() map[string]any {
	si.mu.RLock()
	defer si.mu.RUnlock()

	taskTypes := make(map[string]int)
	projectCounts := make(map[string]int)
	totalMessages := 0

	for _, entry := range si.entries {
		if entry.TaskType != "" {
			taskTypes[entry.TaskType]++
		}
		if entry.ProjectPath != "" {
			projectCounts[entry.ProjectPath]++
		}
		totalMessages += entry.MessageCount
	}

	return map[string]any{
		"total_sessions": len(si.entries),
		"total_messages": totalMessages,
		"total_tags":     len(si.tags),
		"task_types":     taskTypes,
		"project_counts": projectCounts,
	}
}

// Helper functions

// tokenize splits text into words.
func tokenize(text string) []string {
	// Simple tokenization - split on non-alphanumeric
	var words []string
	var current strings.Builder

	for _, r := range text {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			current.WriteRune(r)
		} else if current.Len() > 0 {
			words = append(words, current.String())
			current.Reset()
		}
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}

	return words
}

// isStopWord returns true for common stop words.
func isStopWord(word string) bool {
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"but": true, "is": true, "are": true, "was": true, "were": true,
		"be": true, "been": true, "being": true, "have": true, "has": true,
		"had": true, "do": true, "does": true, "did": true, "will": true,
		"would": true, "could": true, "should": true, "may": true, "might": true,
		"must": true, "can": true, "to": true, "of": true, "in": true,
		"for": true, "on": true, "with": true, "at": true, "by": true,
		"from": true, "as": true, "into": true, "through": true, "during": true,
		"before": true, "after": true, "above": true, "below": true, "between": true,
		"this": true, "that": true, "these": true, "those": true, "it": true,
		"its": true, "you": true, "your": true, "we": true, "our": true,
		"they": true, "their": true, "them": true, "he": true, "she": true,
		"his": true, "her": true, "i": true, "my": true, "me": true,
	}
	return stopWords[strings.ToLower(word)]
}
