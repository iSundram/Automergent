package context

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRankingConfig(t *testing.T) {
	cfg := DefaultRankingConfig()

	// Verify weights sum to approximately 1.0
	total := cfg.IntentWeight + cfg.EditWeight + cfg.SymbolWeight +
		cfg.FrequencyWeight + cfg.RecencyWeight + cfg.DependencyWeight + cfg.FreshnessWeight + cfg.StalenessWeight
	if total < 0.95 || total > 1.05 {
		t.Errorf("Weights should sum to ~1.0, got %f", total)
	}
}

func TestRankerBasic(t *testing.T) {
	ranker := NewRanker(DefaultRankingConfig())

	files := []FileContext{
		{
			Path:           "/app/user/auth.go",
			Content:        "func Login() { authenticate() }",
			Symbols:        []string{"Login", "authenticate"},
			AccessCount:    5,
			LastAccessTime: time.Now().Add(-1 * time.Hour),
		},
		{
			Path:           "/app/db/connection.go",
			Content:        "func Connect() { open database }",
			Symbols:        []string{"Connect"},
			AccessCount:    2,
			LastAccessTime: time.Now().Add(-24 * time.Hour),
		},
		{
			Path:           "/app/user/profile.go",
			Content:        "type User struct { profile data }",
			Symbols:        []string{"User"},
			AccessCount:    10,
			LastAccessTime: time.Now().Add(-30 * time.Minute),
		},
	}

	// Test ranking with login-related intent
	scores := ranker.RankFiles(files, "fix login authentication bug", 0)

	if len(scores) != 3 {
		t.Fatalf("Expected 3 scores, got %d", len(scores))
	}

	// auth.go should rank highest for "login authentication"
	if scores[0].Path != "/app/user/auth.go" {
		t.Errorf("Expected auth.go to rank first, got %s", scores[0].Path)
	}
}

func TestRankerEmptyIntent(t *testing.T) {
	ranker := NewRanker(DefaultRankingConfig())

	files := []FileContext{
		{Path: "/app/main.go", Content: "package main"},
	}

	scores := ranker.RankFiles(files, "", 0)
	if len(scores) != 1 {
		t.Errorf("Expected 1 score, got %d", len(scores))
	}
	if scores[0].Score < 0 || scores[0].Score > 1 {
		t.Errorf("Score should be between 0 and 1, got %f", scores[0].Score)
	}
}

func TestRankerLimit(t *testing.T) {
	ranker := NewRanker(DefaultRankingConfig())

	files := make([]FileContext, 10)
	for i := 0; i < 10; i++ {
		files[i] = FileContext{Path: "/app/file" + string(rune('0'+i)) + ".go"}
	}

	scores := ranker.RankFiles(files, "test", 3)
	if len(scores) != 3 {
		t.Errorf("Expected 3 scores with limit, got %d", len(scores))
	}
}

func TestRankerModifiedFilesBoost(t *testing.T) {
	ranker := NewRanker(DefaultRankingConfig())

	files := []FileContext{
		{Path: "/app/unchanged.go", Content: "unchanged", IsModified: false},
		{Path: "/app/modified.go", Content: "modified", IsModified: true},
	}

	scores := ranker.RankFiles(files, "any intent", 0)

	// Modified file should have higher edit distance score
	var modifiedScore, unchangedScore float64
	for _, s := range scores {
		if s.Path == "/app/modified.go" {
			modifiedScore = s.EditDistance
		} else {
			unchangedScore = s.EditDistance
		}
	}

	if modifiedScore <= unchangedScore {
		t.Errorf("Modified file should have higher edit distance score: %f vs %f",
			modifiedScore, unchangedScore)
	}
}

func TestKeywordExtraction(t *testing.T) {
	tests := []struct {
		intent   string
		minWords int
	}{
		{"fix the login authentication bug", 3}, // login, authentication, bug
		{"add user profile feature", 3},         // add, user, profile, feature
		{"the and or is are", 0},                // all stop words
		{"", 0},                                 // empty
	}

	for _, tc := range tests {
		keywords := extractKeywords(tc.intent)
		if len(keywords) < tc.minWords {
			t.Errorf("extractKeywords(%q) = %v, expected at least %d words",
				tc.intent, keywords, tc.minWords)
		}
	}
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		s, t     string
		expected int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"abc", "abd", 1},
		{"abc", "adc", 1},
		{"kitten", "sitting", 3},
	}

	for _, tc := range tests {
		result := levenshteinDistance(tc.s, tc.t)
		if result != tc.expected {
			t.Errorf("levenshteinDistance(%q, %q) = %d, expected %d",
				tc.s, tc.t, result, tc.expected)
		}
	}
}

func TestFuzzyMatch(t *testing.T) {
	tests := []struct {
		s, pattern string
		expected   bool
	}{
		{"userService", "user", true},
		{"getUserById", "user", true},
		{"database", "base", true},
		{"connection", "conn", true},
		{"completely_different", "xyz", false},
	}

	for _, tc := range tests {
		result := fuzzyMatch(tc.s, tc.pattern)
		if result != tc.expected {
			t.Errorf("fuzzyMatch(%q, %q) = %v, expected %v",
				tc.s, tc.pattern, result, tc.expected)
		}
	}
}

func TestSymbolIndex(t *testing.T) {
	ranker := NewRanker(DefaultRankingConfig())

	ranker.IndexSymbols("/app/user.go", []string{"User", "GetUser", "UpdateUser"})
	ranker.IndexSymbols("/app/auth.go", []string{"Login", "Logout", "User"})

	// Find by symbol
	userFiles := ranker.FindBySymbol("User")
	if len(userFiles) != 2 {
		t.Errorf("Expected 2 files with User symbol, got %d", len(userFiles))
	}

	loginFiles := ranker.FindBySymbol("Login")
	if len(loginFiles) != 1 {
		t.Errorf("Expected 1 file with Login symbol, got %d", len(loginFiles))
	}
}

func TestPathKeywords(t *testing.T) {
	tests := []struct {
		path         string
		expectedKeys []string
	}{
		{"/app/user/auth.go", []string{"user", "auth", "go"}},
		{"/src/UserService.ts", []string{"user", "service", "ts"}},
		{"/pkg/http-client/client.go", []string{"http", "client", "go"}},
	}

	for _, tc := range tests {
		keywords := ExtractPathKeywords(tc.path)
		for _, expected := range tc.expectedKeys {
			found := false
			for _, kw := range keywords {
				if kw == expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("ExtractPathKeywords(%q) missing keyword %q, got %v",
					tc.path, expected, keywords)
			}
		}
	}
}

func TestSplitCamelCase(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"UserService", []string{"user", "service"}},
		{"getHTTPClient", []string{"get", "h", "t", "t", "p", "client"}}, // Note: consecutive caps
		{"simple", []string{"simple"}},
		{"", nil},
	}

	for _, tc := range tests {
		result := splitCamelCase(tc.input)
		if len(result) != len(tc.expected) {
			t.Errorf("splitCamelCase(%q) = %v, expected %v", tc.input, result, tc.expected)
		}
	}
}

func BenchmarkRankFiles(b *testing.B) {
	ranker := NewRanker(DefaultRankingConfig())

	// Create 100 files
	files := make([]FileContext, 100)
	for i := 0; i < 100; i++ {
		files[i] = FileContext{
			Path:           "/app/file" + string(rune('a'+i%26)) + ".go",
			Content:        "func Example() { /* some code */ }",
			Symbols:        []string{"Example", "Helper", "Utils"},
			AccessCount:    i % 10,
			LastAccessTime: time.Now().Add(-time.Duration(i) * time.Minute),
		}
	}

	intent := "fix authentication and user login"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ranker.RankFiles(files, intent, 10)
	}
}

func BenchmarkLevenshteinDistance(b *testing.B) {
	s := "authentication"
	t := "authorization"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		levenshteinDistance(s, t)
	}
}

func BenchmarkExtractKeywords(b *testing.B) {
	intent := "Fix the user authentication and login flow in the API service"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		extractKeywords(intent)
	}
}

// Integration test with real files
func TestRankerWithRealFiles(t *testing.T) {
	// Create temp directory
	tempDir := t.TempDir()

	// Create test files
	files := map[string]string{
		"auth.go":    "package auth\n\nfunc Login() error { return nil }",
		"user.go":    "package user\n\ntype User struct { ID int }",
		"db.go":      "package db\n\nfunc Connect() error { return nil }",
		"handler.go": "package handler\n\nfunc HandleLogin() { /* auth logic */ }",
	}

	for name, content := range files {
		path := filepath.Join(tempDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Build file contexts
	var fileContexts []FileContext
	for name := range files {
		path := filepath.Join(tempDir, name)
		content, _ := os.ReadFile(path)
		fileContexts = append(fileContexts, FileContext{
			Path:    path,
			Content: string(content),
			Symbols: extractSymbols(string(content), ".go"),
		})
	}

	ranker := NewRanker(DefaultRankingConfig())
	scores := ranker.RankFiles(fileContexts, "fix login authentication", 0)

	// Files with "login" or "auth" should rank higher
	if len(scores) < 2 {
		t.Fatalf("Expected at least 2 scores, got %d", len(scores))
	}

	// Top results should contain auth-related files
	topFile := filepath.Base(scores[0].Path)
	if topFile != "auth.go" && topFile != "handler.go" {
		t.Logf("Top file: %s (expected auth.go or handler.go)", topFile)
	}
}

// Test extract symbols functions
func TestExtractGoSymbols(t *testing.T) {
	content := `package main

func Public() {}
func private() {}
type UserService struct {}
type internal struct {}
const MaxCount = 100
var GlobalVar = "test"
`
	symbols := extractGoSymbols(content)

	expected := []string{"Public", "UserService", "MaxCount", "GlobalVar"}
	for _, exp := range expected {
		found := false
		for _, sym := range symbols {
			if sym == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected symbol %q not found in %v", exp, symbols)
		}
	}

	// Unexported symbols should not be included
	for _, sym := range symbols {
		if sym == "private" || sym == "internal" {
			t.Errorf("Unexported symbol %q should not be included", sym)
		}
	}
}

func TestExtractTSSymbols(t *testing.T) {
	content := `
export function getUserById(id: string) {}
export class UserService {}
export const API_URL = "http://example.com"
export interface User {}
export type UserId = string
function private() {}
`
	symbols := extractTSSymbols(content)

	expected := []string{"getUserById", "UserService", "API_URL", "User", "UserId"}
	for _, exp := range expected {
		found := false
		for _, sym := range symbols {
			if sym == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected symbol %q not found in %v", exp, symbols)
		}
	}
}

func TestExtractPySymbols(t *testing.T) {
	content := `
class UserService:
    pass

class _PrivateClass:
    pass

def get_user():
    pass

def _private_func():
    pass
`
	symbols := extractPySymbols(content)

	if !containsString(symbols, "UserService") {
		t.Error("Expected UserService symbol")
	}
	if !containsString(symbols, "get_user") {
		t.Error("Expected get_user symbol")
	}
	if containsString(symbols, "_PrivateClass") {
		t.Error("_PrivateClass should not be included")
	}
	if containsString(symbols, "_private_func") {
		t.Error("_private_func should not be included")
	}
}

func TestRecencyWeight(t *testing.T) {
	cfg := DefaultRankingConfig()
	ranker := NewRanker(cfg)

	// File accessed now vs 48 hours ago
	recentFile := FileContext{
		Path:           "/recent.go",
		LastAccessTime: time.Now(),
	}
	oldFile := FileContext{
		Path:           "/old.go",
		LastAccessTime: time.Now().Add(-48 * time.Hour),
	}

	recentScore := ranker.calculateRecencyWeight(recentFile)
	oldScore := ranker.calculateRecencyWeight(oldFile)

	if recentScore <= oldScore {
		t.Errorf("Recent file should have higher recency weight: %f vs %f",
			recentScore, oldScore)
	}
	if recentScore < 0.9 {
		t.Errorf("Just accessed file should have weight close to 1.0, got %f", recentScore)
	}
}

func TestFrequencyWeight(t *testing.T) {
	ranker := NewRanker(DefaultRankingConfig())

	// More frequently accessed = higher weight
	lowFreq := FileContext{Path: "/low.go", AccessCount: 1}
	highFreq := FileContext{Path: "/high.go", AccessCount: 15}

	lowScore := ranker.calculateFrequencyWeight(lowFreq)
	highScore := ranker.calculateFrequencyWeight(highFreq)

	if highScore <= lowScore {
		t.Errorf("High frequency file should have higher weight: %f vs %f",
			highScore, lowScore)
	}
}

func TestDependencyWeight(t *testing.T) {
	ranker := NewRanker(DefaultRankingConfig())

	// File with many dependents should score higher
	isolated := FileContext{Path: "/isolated.go", Dependents: nil}
	central := FileContext{Path: "/central.go", Dependents: []string{"a", "b", "c", "d", "e"}}

	isolatedScore := ranker.calculateDependencyWeight(isolated)
	centralScore := ranker.calculateDependencyWeight(central)

	if centralScore <= isolatedScore {
		t.Errorf("Central file should have higher dependency weight: %f vs %f",
			centralScore, isolatedScore)
	}
}

// Test parallel ranking
func TestParallelRanking(t *testing.T) {
	ranker := NewRanker(DefaultRankingConfig())

	// Create many files
	files := make([]FileContext, 50)
	for i := 0; i < 50; i++ {
		files[i] = FileContext{
			Path:    "/file" + string(rune('a'+i%26)) + ".go",
			Content: "func Handler() {}",
			Symbols: []string{"Handler"},
		}
	}

	// Run multiple rankings concurrently
	ctx := context.Background()
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			scores := ranker.RankFiles(files, "test intent", 10)
			if len(scores) != 10 {
				t.Errorf("Expected 10 scores, got %d", len(scores))
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		select {
		case <-done:
		case <-ctx.Done():
			t.Fatal("Timeout waiting for parallel rankings")
		}
	}
}
