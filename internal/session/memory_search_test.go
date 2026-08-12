package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMemoryStore_Basic(t *testing.T) {
	dir := t.TempDir()
	ms, err := NewMemoryStore(dir)
	if err != nil {
		t.Fatalf("NewMemoryStore: %v", err)
	}

	projectPath := "/home/user/myproject"
	pm, err := ms.GetProject(projectPath)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}

	if pm.ProjectPath != projectPath {
		t.Errorf("ProjectPath = %q, want %q", pm.ProjectPath, projectPath)
	}
	if pm.ProjectName != "myproject" {
		t.Errorf("ProjectName = %q, want %q", pm.ProjectName, "myproject")
	}
}

func TestMemoryStore_LearnConvention(t *testing.T) {
	dir := t.TempDir()
	ms, err := NewMemoryStore(dir)
	if err != nil {
		t.Fatalf("NewMemoryStore: %v", err)
	}

	projectPath := "/home/user/project"
	err = ms.LearnConvention(projectPath, "naming", "Use camelCase for variables", 0.9)
	if err != nil {
		t.Fatalf("LearnConvention: %v", err)
	}

	pm, _ := ms.GetProject(projectPath)
	conv, ok := pm.Conventions["naming"]
	if !ok {
		t.Fatal("Convention not found")
	}
	if conv.Description != "Use camelCase for variables" {
		t.Errorf("Description = %q, want %q", conv.Description, "Use camelCase for variables")
	}
	if conv.Confidence != 0.9 {
		t.Errorf("Confidence = %f, want 0.9", conv.Confidence)
	}

	// Learn again to update confidence
	err = ms.LearnConvention(projectPath, "naming", "Use camelCase for variables", 1.0)
	if err != nil {
		t.Fatalf("LearnConvention again: %v", err)
	}

	pm, _ = ms.GetProject(projectPath)
	conv = pm.Conventions["naming"]
	if conv.Confidence != 0.95 { // (0.9 + 1.0) / 2
		t.Errorf("Confidence after update = %f, want 0.95", conv.Confidence)
	}
}

func TestMemoryStore_LearnCodePattern(t *testing.T) {
	dir := t.TempDir()
	ms, err := NewMemoryStore(dir)
	if err != nil {
		t.Fatalf("NewMemoryStore: %v", err)
	}

	projectPath := "/home/user/project"
	pattern := CodePattern{
		Name:        "error_handling",
		Description: "Standard error handling pattern",
		Pattern:     "if err != nil { return err }",
		Language:    "go",
		Tags:        []string{"error", "go"},
	}

	err = ms.LearnCodePattern(projectPath, pattern)
	if err != nil {
		t.Fatalf("LearnCodePattern: %v", err)
	}

	pm, _ := ms.GetProject(projectPath)
	if len(pm.CodePatterns) != 1 {
		t.Fatalf("len(CodePatterns) = %d, want 1", len(pm.CodePatterns))
	}
	if pm.CodePatterns[0].Name != "error_handling" {
		t.Errorf("Pattern name = %q, want %q", pm.CodePatterns[0].Name, "error_handling")
	}
}

func TestMemoryStore_RecordFileAccess(t *testing.T) {
	dir := t.TempDir()
	ms, err := NewMemoryStore(dir)
	if err != nil {
		t.Fatalf("NewMemoryStore: %v", err)
	}

	projectPath := "/home/user/project"

	// Record access to multiple files
	_ = ms.RecordFileAccess(projectPath, "main.go", true)
	_ = ms.RecordFileAccess(projectPath, "utils.go", false)
	_ = ms.RecordFileAccess(projectPath, "main.go", true) // Access again

	pm, _ := ms.GetProject(projectPath)
	if len(pm.RecentFiles) != 2 {
		t.Fatalf("len(RecentFiles) = %d, want 2", len(pm.RecentFiles))
	}

	// main.go should be first (most recent)
	if pm.RecentFiles[0].Path != "main.go" {
		t.Errorf("First file = %q, want main.go", pm.RecentFiles[0].Path)
	}
	if pm.RecentFiles[0].EditCount != 2 {
		t.Errorf("EditCount = %d, want 2", pm.RecentFiles[0].EditCount)
	}
}

func TestMemoryStore_SetArchitecture(t *testing.T) {
	dir := t.TempDir()
	ms, err := NewMemoryStore(dir)
	if err != nil {
		t.Fatalf("NewMemoryStore: %v", err)
	}

	projectPath := "/home/user/project"
	arch := &ArchitectureDecisions{
		Style:         "modular",
		BuildSystem:   "make",
		TestFramework: "go test",
		Directories: map[string]string{
			"commands": "cmd",
			"internal": "internal",
		},
	}

	err = ms.SetArchitecture(projectPath, arch)
	if err != nil {
		t.Fatalf("SetArchitecture: %v", err)
	}

	pm, _ := ms.GetProject(projectPath)
	if pm.Architecture == nil {
		t.Fatal("Architecture is nil")
	}
	if pm.Architecture.Style != "modular" {
		t.Errorf("Style = %q, want %q", pm.Architecture.Style, "modular")
	}
}

func TestMemoryStore_RecentProjects(t *testing.T) {
	dir := t.TempDir()
	ms, err := NewMemoryStore(dir)
	if err != nil {
		t.Fatalf("NewMemoryStore: %v", err)
	}

	// Access multiple projects
	_, _ = ms.GetProject("/home/user/project1")
	_, _ = ms.GetProject("/home/user/project2")
	_, _ = ms.GetProject("/home/user/project3")

	recent := ms.GetRecentProjects(10)
	if len(recent) != 3 {
		t.Fatalf("len(recent) = %d, want 3", len(recent))
	}

	// Most recent should be first
	if recent[0].Path != "/home/user/project3" {
		t.Errorf("First recent = %q, want /home/user/project3", recent[0].Path)
	}
}

func TestMemoryStore_UserPreferences(t *testing.T) {
	dir := t.TempDir()
	ms, err := NewMemoryStore(dir)
	if err != nil {
		t.Fatalf("NewMemoryStore: %v", err)
	}

	err = ms.SetUserPreference("theme", "dark")
	if err != nil {
		t.Fatalf("SetUserPreference: %v", err)
	}

	val, ok := ms.GetUserPreference("theme")
	if !ok {
		t.Fatal("Preference not found")
	}
	if val != "dark" {
		t.Errorf("Value = %q, want %q", val, "dark")
	}

	// Verify persistence
	ms2, _ := NewMemoryStore(dir)
	val, ok = ms2.GetUserPreference("theme")
	if !ok || val != "dark" {
		t.Error("Preference not persisted")
	}
}

func TestMemoryStore_MostUsedPatterns(t *testing.T) {
	dir := t.TempDir()
	ms, err := NewMemoryStore(dir)
	if err != nil {
		t.Fatalf("NewMemoryStore: %v", err)
	}

	projectPath := "/home/user/project"

	// Add patterns with different usage counts
	for i := 0; i < 3; i++ {
		p := CodePattern{Name: "pattern" + string(rune('A'+i))}
		_ = ms.LearnCodePattern(projectPath, p)
	}

	pm, _ := ms.GetProject(projectPath)
	// Manually set usage counts for testing
	pm.CodePatterns[0].UsageCount = 5
	pm.CodePatterns[1].UsageCount = 10
	pm.CodePatterns[2].UsageCount = 3
	_ = ms.SaveProject(pm)

	patterns := ms.GetMostUsedPatterns(projectPath, 2)
	if len(patterns) != 2 {
		t.Fatalf("len(patterns) = %d, want 2", len(patterns))
	}
	if patterns[0].UsageCount != 10 {
		t.Errorf("First pattern usage = %d, want 10", patterns[0].UsageCount)
	}
}

func TestMemoryStore_Persistence(t *testing.T) {
	dir := t.TempDir()

	// Create and populate
	ms, err := NewMemoryStore(dir)
	if err != nil {
		t.Fatalf("NewMemoryStore: %v", err)
	}

	projectPath := "/home/user/project"
	_ = ms.LearnConvention(projectPath, "test_conv", "Test description", 0.8)
	_ = ms.SaveAll()

	// Load in new instance
	ms2, err := NewMemoryStore(dir)
	if err != nil {
		t.Fatalf("NewMemoryStore: %v", err)
	}

	pm, _ := ms2.GetProject(projectPath)
	if _, ok := pm.Conventions["test_conv"]; !ok {
		t.Error("Convention not persisted")
	}
}

func TestMemoryStore_IncrementPatternUsage(t *testing.T) {
	dir := t.TempDir()
	ms, err := NewMemoryStore(dir)
	if err != nil {
		t.Fatalf("NewMemoryStore: %v", err)
	}

	projectPath := "/home/user/project"
	pattern := CodePattern{Name: "test_pattern"}
	_ = ms.LearnCodePattern(projectPath, pattern)

	pm, _ := ms.GetProject(projectPath)
	patternID := pm.CodePatterns[0].ID

	originalUsage := pm.CodePatterns[0].UsageCount
	_ = ms.IncrementPatternUsage(projectPath, patternID)

	pm, _ = ms.GetProject(projectPath)
	if pm.CodePatterns[0].UsageCount != originalUsage+1 {
		t.Errorf("UsageCount = %d, want %d", pm.CodePatterns[0].UsageCount, originalUsage+1)
	}
}

func TestSearchIndex_IndexAndSearch(t *testing.T) {
	dir := t.TempDir()
	si, err := NewSearchIndex(dir)
	if err != nil {
		t.Fatalf("NewSearchIndex: %v", err)
	}

	sess := New()
	sess.Title = "Fix authentication bug"
	sess.AddMessage(newTestMessage("Fix the login authentication error"))

	state := &PersistenceState{
		ProjectPath: "/home/user/project",
		WorkDir:     "/home/user/project/src",
	}

	if err := si.IndexSession(sess, state); err != nil {
		t.Fatalf("IndexSession: %v", err)
	}

	// Search by text
	results := si.Search(SearchQuery{Text: "authentication"})
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Entry.SessionID != sess.ID {
		t.Errorf("SessionID = %q, want %q", results[0].Entry.SessionID, sess.ID)
	}
	if results[0].Entry.TaskType != "debug" { // Should infer "fix" -> "debug"
		t.Errorf("TaskType = %q, want debug", results[0].Entry.TaskType)
	}
}

func TestSearchIndex_SearchByProject(t *testing.T) {
	dir := t.TempDir()
	si, err := NewSearchIndex(dir)
	if err != nil {
		t.Fatalf("NewSearchIndex: %v", err)
	}

	// Index sessions for different projects
	sess1 := New()
	sess1.Title = "Session 1"
	state1 := &PersistenceState{ProjectPath: "/project1"}
	_ = si.IndexSession(sess1, state1)

	sess2 := New()
	sess2.Title = "Session 2"
	state2 := &PersistenceState{ProjectPath: "/project2"}
	_ = si.IndexSession(sess2, state2)

	// Search for project1 only
	results := si.Search(SearchQuery{ProjectPath: "/project1"})
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Entry.SessionID != sess1.ID {
		t.Errorf("Wrong session returned")
	}
}

func TestSearchIndex_Tags(t *testing.T) {
	dir := t.TempDir()
	si, err := NewSearchIndex(dir)
	if err != nil {
		t.Fatalf("NewSearchIndex: %v", err)
	}

	sess := New()
	sess.Title = "Tagged Session"
	_ = si.IndexSession(sess, nil)

	// Add tags
	if err := si.AddTag(sess.ID, "important"); err != nil {
		t.Fatalf("AddTag: %v", err)
	}
	if err := si.AddTag(sess.ID, "bug"); err != nil {
		t.Fatalf("AddTag: %v", err)
	}

	// Get by tag
	tagged := si.GetByTag("important")
	if len(tagged) != 1 {
		t.Fatalf("len(tagged) = %d, want 1", len(tagged))
	}

	// Search by tag
	results := si.Search(SearchQuery{Tags: []string{"bug"}})
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}

	// Remove tag
	if err := si.RemoveTag(sess.ID, "important"); err != nil {
		t.Fatalf("RemoveTag: %v", err)
	}
	tagged = si.GetByTag("important")
	if len(tagged) != 0 {
		t.Errorf("len(tagged) after remove = %d, want 0", len(tagged))
	}
}

func TestSearchIndex_FindSimilar(t *testing.T) {
	dir := t.TempDir()
	si, err := NewSearchIndex(dir)
	if err != nil {
		t.Fatalf("NewSearchIndex: %v", err)
	}

	// Index multiple sessions with shared characteristics
	sess1 := New()
	sess1.Title = "Fix auth bug"
	sess1.AddMessage(newTestMessage("Fix login error"))
	state1 := &PersistenceState{ProjectPath: "/myproject"}
	_ = si.IndexSession(sess1, state1)

	sess2 := New()
	sess2.Title = "Another auth fix"
	sess2.AddMessage(newTestMessage("Fix authentication"))
	state2 := &PersistenceState{ProjectPath: "/myproject"}
	_ = si.IndexSession(sess2, state2)

	sess3 := New()
	sess3.Title = "Unrelated task"
	sess3.AddMessage(newTestMessage("Add new feature"))
	state3 := &PersistenceState{ProjectPath: "/otherproject"}
	_ = si.IndexSession(sess3, state3)

	similar := si.FindSimilar(sess1.ID, 5)
	if len(similar) < 1 {
		t.Fatal("No similar sessions found")
	}

	// sess2 should be more similar than sess3 (same project)
	if similar[0].Entry.SessionID != sess2.ID {
		t.Errorf("Most similar = %q, want %q", similar[0].Entry.SessionID, sess2.ID)
	}
}

func TestSearchIndex_Stats(t *testing.T) {
	dir := t.TempDir()
	si, err := NewSearchIndex(dir)
	if err != nil {
		t.Fatalf("NewSearchIndex: %v", err)
	}

	sess1 := New()
	sess1.Title = "Fix bug"
	sess1.AddMessage(newTestMessage("Fix bug"))
	sess1.AddMessage(newTestMessage("Done"))
	_ = si.IndexSession(sess1, nil)

	sess2 := New()
	sess2.Title = "Add feature"
	sess2.AddMessage(newTestMessage("Add feature"))
	_ = si.IndexSession(sess2, nil)

	stats := si.Stats()
	if stats["total_sessions"].(int) != 2 {
		t.Errorf("total_sessions = %v, want 2", stats["total_sessions"])
	}
	if stats["total_messages"].(int) != 3 {
		t.Errorf("total_messages = %v, want 3", stats["total_messages"])
	}
}

func TestSearchIndex_Delete(t *testing.T) {
	dir := t.TempDir()
	si, err := NewSearchIndex(dir)
	if err != nil {
		t.Fatalf("NewSearchIndex: %v", err)
	}

	sess := New()
	_ = si.IndexSession(sess, nil)

	_, ok := si.GetEntry(sess.ID)
	if !ok {
		t.Fatal("Entry not found after indexing")
	}

	if err := si.Delete(sess.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, ok = si.GetEntry(sess.ID)
	if ok {
		t.Error("Entry still exists after delete")
	}
}

func TestSearchIndex_TimeRange(t *testing.T) {
	dir := t.TempDir()
	si, err := NewSearchIndex(dir)
	if err != nil {
		t.Fatalf("NewSearchIndex: %v", err)
	}

	// Create sessions at different times
	now := time.Now()

	sess1 := New()
	sess1.Title = "Recent"
	sess1.CreatedAt = now
	_ = si.IndexSession(sess1, nil)

	sess2 := New()
	sess2.Title = "Old"
	sess2.CreatedAt = now.Add(-48 * time.Hour)
	_ = si.IndexSession(sess2, nil)

	// Search for recent only
	results := si.Search(SearchQuery{
		TimeRange: &TimeRange{
			After: now.Add(-24 * time.Hour),
		},
	})
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Entry.SessionID != sess1.ID {
		t.Error("Wrong session returned for time range")
	}
}

func TestPreloader_Basic(t *testing.T) {
	dir := t.TempDir()
	ms, _ := NewMemoryStore(dir)
	si, _ := NewSearchIndex(dir)

	preloader := NewPreloader(ms, si, DefaultPreloadConfig())

	projectPath := t.TempDir()

	// Create some files
	_ = os.WriteFile(filepath.Join(projectPath, "go.mod"), []byte("module test"), 0644)
	_ = os.WriteFile(filepath.Join(projectPath, "main.go"), []byte("package main"), 0644)

	ctx, err := preloader.PreloadProject(projectPath, DefaultPreloadConfig())
	if err != nil {
		t.Fatalf("PreloadProject: %v", err)
	}

	if ctx.ProjectPath != projectPath {
		t.Errorf("ProjectPath = %q, want %q", ctx.ProjectPath, projectPath)
	}

	// Should have predicted files
	if len(ctx.PredictedFiles) == 0 {
		t.Error("No predicted files")
	}
}

func TestPreloader_Cache(t *testing.T) {
	dir := t.TempDir()
	ms, _ := NewMemoryStore(dir)
	si, _ := NewSearchIndex(dir)

	config := PreloadConfig{
		CacheExpiry:   1 * time.Hour,
		MaxCacheItems: 5,
	}
	preloader := NewPreloader(ms, si, config)

	projectPath := t.TempDir()
	_ = os.WriteFile(filepath.Join(projectPath, "main.go"), []byte("package main"), 0644)

	// First load
	ctx1, _ := preloader.PreloadProject(projectPath, config)

	// Should be cached
	ctx2, ok := preloader.GetCachedContext(projectPath)
	if !ok {
		t.Fatal("Context not cached")
	}
	if ctx2.LoadedAt != ctx1.LoadedAt {
		t.Error("Cache returned different context")
	}

	// Invalidate
	preloader.InvalidateCache(projectPath)
	_, ok = preloader.GetCachedContext(projectPath)
	if ok {
		t.Error("Context still cached after invalidation")
	}
}

func TestPreloader_SmartDefaults(t *testing.T) {
	dir := t.TempDir()
	ms, _ := NewMemoryStore(dir)
	si, _ := NewSearchIndex(dir)

	preloader := NewPreloader(ms, si, DefaultPreloadConfig())

	projectPath := t.TempDir()

	// Create Go project indicators
	_ = os.WriteFile(filepath.Join(projectPath, "go.mod"), []byte("module test"), 0644)
	_ = os.WriteFile(filepath.Join(projectPath, "Makefile"), []byte("build:"), 0644)

	defaults := preloader.SmartDefaults(projectPath)

	if defaults["project_type"] != "go" {
		t.Errorf("project_type = %q, want %q", defaults["project_type"], "go")
	}
	if defaults["build_system"] != "make" && defaults["build_system"] != "go" {
		t.Errorf("build_system = %q, want make or go", defaults["build_system"])
	}
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"hello world", []string{"hello", "world"}},
		{"fix_the_bug", []string{"fix_the_bug"}},
		{"camelCase", []string{"camelCase"}},
		{"with-dashes", []string{"with", "dashes"}},
		{"path/to/file.go", []string{"path", "to", "file", "go"}},
	}

	for _, tt := range tests {
		result := tokenize(tt.input)
		if len(result) != len(tt.expected) {
			t.Errorf("tokenize(%q) = %v, want %v", tt.input, result, tt.expected)
			continue
		}
		for i, word := range result {
			if word != tt.expected[i] {
				t.Errorf("tokenize(%q)[%d] = %q, want %q", tt.input, i, word, tt.expected[i])
			}
		}
	}
}

func TestIsStopWord(t *testing.T) {
	stopWords := []string{"the", "a", "is", "and", "or", "but"}
	for _, word := range stopWords {
		if !isStopWord(word) {
			t.Errorf("isStopWord(%q) = false, want true", word)
		}
	}

	nonStopWords := []string{"function", "error", "code", "test"}
	for _, word := range nonStopWords {
		if isStopWord(word) {
			t.Errorf("isStopWord(%q) = true, want false", word)
		}
	}
}
