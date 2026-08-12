package context

import (
	"math"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// RelevanceScore represents the calculated relevance of a file.
type RelevanceScore struct {
	Path              string  `json:"path"`
	Score             float64 `json:"score"`
	IntentAlignment   float64 `json:"intent_alignment"`
	EditDistance      float64 `json:"edit_distance"`
	SymbolRelevance   float64 `json:"symbol_relevance"`
	FrequencyWeight   float64 `json:"frequency_weight"`
	RecencyWeight     float64 `json:"recency_weight"`
	DependencyWeight  float64 `json:"dependency_weight"`
	FreshnessWeight   float64 `json:"freshness_weight"`
	StalenessDiscount float64 `json:"staleness_discount"`
}

// RankingConfig configures the semantic ranking algorithm.
type RankingConfig struct {
	// Weight factors for scoring components (0.0 - 1.0)
	IntentWeight     float64 `json:"intent_weight"`
	EditWeight       float64 `json:"edit_weight"`
	SymbolWeight     float64 `json:"symbol_weight"`
	FrequencyWeight  float64 `json:"frequency_weight"`
	RecencyWeight    float64 `json:"recency_weight"`
	DependencyWeight float64 `json:"dependency_weight"`
	FreshnessWeight  float64 `json:"freshness_weight"`
	StalenessWeight  float64 `json:"staleness_weight"`

	// Decay settings
	RecencyDecayHours float64 `json:"recency_decay_hours"`
	FrequencyDecayMax int     `json:"frequency_decay_max"`
}

// DefaultRankingConfig returns optimized ranking weights.
func DefaultRankingConfig() RankingConfig {
	return RankingConfig{
		IntentWeight:      0.23,
		EditWeight:        0.12,
		SymbolWeight:      0.18,
		FrequencyWeight:   0.08,
		RecencyWeight:     0.12,
		DependencyWeight:  0.10,
		FreshnessWeight:   0.12,
		StalenessWeight:   0.05,
		RecencyDecayHours: 24.0,
		FrequencyDecayMax: 20,
	}
}

// FileContext holds metadata about a file for ranking.
type FileContext struct {
	Path           string
	Content        string
	Symbols        []string
	Imports        []string
	AccessCount    int
	LastAccessTime time.Time
	ModTime        time.Time
	IsModified     bool // Whether file has uncommitted changes
	Dependencies   []string
	Dependents     []string
	Freshness      float64
	Staleness      float64
	FreshnessState string
}

// Ranker performs semantic relevance ranking of files.
type Ranker struct {
	config  RankingConfig
	mu      sync.RWMutex
	cache   map[string]*scoreCacheEntry
	symbols *symbolIndex
}

type scoreCacheEntry struct {
	score    RelevanceScore
	computed time.Time
}

type symbolIndex struct {
	mu       sync.RWMutex
	symbols  map[string][]string // symbol -> file paths
	paths    map[string][]string // file path -> symbols
	keywords map[string][]string // keyword -> file paths
}

// NewRanker creates a new semantic ranker.
func NewRanker(cfg RankingConfig) *Ranker {
	return &Ranker{
		config: cfg,
		cache:  make(map[string]*scoreCacheEntry),
		symbols: &symbolIndex{
			symbols:  make(map[string][]string),
			paths:    make(map[string][]string),
			keywords: make(map[string][]string),
		},
	}
}

// RankFiles ranks files by relevance to the given intent and returns sorted results.
func (r *Ranker) RankFiles(files []FileContext, intent string, limit int) []RelevanceScore {
	if len(files) == 0 {
		return nil
	}

	// Parse intent into keywords
	intentKeywords := extractKeywords(intent)

	// Calculate scores in parallel
	scores := make([]RelevanceScore, len(files))
	var wg sync.WaitGroup
	wg.Add(len(files))

	for i, f := range files {
		go func(idx int, fc FileContext) {
			defer wg.Done()
			scores[idx] = r.calculateScore(fc, intentKeywords)
		}(i, f)
	}

	wg.Wait()

	// Sort by score descending
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})

	// Apply limit
	if limit > 0 && len(scores) > limit {
		scores = scores[:limit]
	}

	return scores
}

// calculateScore computes the composite relevance score for a file.
func (r *Ranker) calculateScore(f FileContext, intentKeywords []string) RelevanceScore {
	score := RelevanceScore{Path: f.Path}

	// 1. Intent alignment - how well does the file match the user's intent?
	score.IntentAlignment = r.calculateIntentAlignment(f, intentKeywords)

	// 2. Edit distance - similarity to recently edited files
	score.EditDistance = r.calculateEditDistanceScore(f)

	// 3. Symbol relevance - relevant symbols (functions, classes, etc.)
	score.SymbolRelevance = r.calculateSymbolRelevance(f, intentKeywords)

	// 4. Frequency weighting - how often is this file accessed?
	score.FrequencyWeight = r.calculateFrequencyWeight(f)

	// 5. Recency weighting - when was this file last accessed?
	score.RecencyWeight = r.calculateRecencyWeight(f)

	// 6. Dependency weighting - is this file a dependency of active files?
	score.DependencyWeight = r.calculateDependencyWeight(f)

	// 7. Freshness / staleness signals
	score.FreshnessWeight = r.calculateFreshnessWeight(f)
	score.StalenessDiscount = r.calculateStalenessDiscount(f)

	// Compute weighted sum
	score.Score = r.config.IntentWeight*score.IntentAlignment +
		r.config.EditWeight*score.EditDistance +
		r.config.SymbolWeight*score.SymbolRelevance +
		r.config.FrequencyWeight*score.FrequencyWeight +
		r.config.RecencyWeight*score.RecencyWeight +
		r.config.DependencyWeight*score.DependencyWeight -
		r.config.FreshnessWeight*score.FreshnessWeight -
		r.config.StalenessWeight*score.StalenessDiscount

	// Normalize to 0-1 range
	score.Score = math.Max(0, math.Min(1, score.Score))

	return score
}

// calculateIntentAlignment scores how well file matches the intent.
func (r *Ranker) calculateIntentAlignment(f FileContext, keywords []string) float64 {
	if len(keywords) == 0 {
		return 0.5 // neutral score
	}

	matches := 0
	content := strings.ToLower(f.Content)
	path := strings.ToLower(f.Path)

	for _, kw := range keywords {
		kwLower := strings.ToLower(kw)
		// Check file path (higher weight)
		if strings.Contains(path, kwLower) {
			matches += 2
		}
		// Check symbols (medium weight)
		for _, sym := range f.Symbols {
			if strings.Contains(strings.ToLower(sym), kwLower) {
				matches++
				break
			}
		}
		// Check content (lower weight)
		if strings.Contains(content, kwLower) {
			matches++
		}
	}

	// Normalize: max score is 4 * len(keywords)
	maxScore := 4.0 * float64(len(keywords))
	return float64(matches) / maxScore
}

// calculateEditDistanceScore computes normalized edit distance score.
func (r *Ranker) calculateEditDistanceScore(f FileContext) float64 {
	// If file has been modified (uncommitted changes), boost score
	if f.IsModified {
		return 1.0
	}
	return 0.3 // base score for unmodified files
}

// calculateSymbolRelevance scores based on symbol matches.
func (r *Ranker) calculateSymbolRelevance(f FileContext, keywords []string) float64 {
	if len(f.Symbols) == 0 || len(keywords) == 0 {
		return 0.0
	}

	matchCount := 0
	for _, sym := range f.Symbols {
		symLower := strings.ToLower(sym)
		for _, kw := range keywords {
			if fuzzyMatch(symLower, strings.ToLower(kw)) {
				matchCount++
				break
			}
		}
	}

	// Normalize by number of symbols (capped)
	return math.Min(1.0, float64(matchCount)/float64(min(len(f.Symbols), 10)))
}

// calculateFrequencyWeight scores based on access frequency.
func (r *Ranker) calculateFrequencyWeight(f FileContext) float64 {
	if f.AccessCount <= 0 {
		return 0.0
	}
	// Logarithmic decay to prevent outliers from dominating
	maxCount := float64(r.config.FrequencyDecayMax)
	return math.Log1p(float64(f.AccessCount)) / math.Log1p(maxCount)
}

// calculateRecencyWeight scores based on how recently file was accessed.
func (r *Ranker) calculateRecencyWeight(f FileContext) float64 {
	if f.LastAccessTime.IsZero() {
		return 0.0
	}
	hoursSince := time.Since(f.LastAccessTime).Hours()
	if hoursSince <= 0 {
		return 1.0
	}
	// Exponential decay
	decay := math.Exp(-hoursSince / r.config.RecencyDecayHours)
	return decay
}

// calculateDependencyWeight scores based on dependency relationships.
func (r *Ranker) calculateDependencyWeight(f FileContext) float64 {
	// More dependents = more important
	dependentScore := math.Min(1.0, float64(len(f.Dependents))/5.0)
	// Being a dependency also matters
	dependencyScore := math.Min(1.0, float64(len(f.Dependencies))/10.0) * 0.5

	return (dependentScore + dependencyScore) / 1.5
}

// calculateFreshnessWeight rewards fresh files.
func (r *Ranker) calculateFreshnessWeight(f FileContext) float64 {
	if f.Freshness > 0 {
		return math.Min(1.0, f.Freshness)
	}
	if f.ModTime.IsZero() {
		return 0.0
	}
	hoursSince := time.Since(f.ModTime).Hours()
	if hoursSince <= 0 {
		return 1.0
	}
	return math.Max(0.0, 1.0-math.Min(1.0, hoursSince/(r.config.RecencyDecayHours*2)))
}

// calculateStalenessDiscount penalizes stale files.
func (r *Ranker) calculateStalenessDiscount(f FileContext) float64 {
	if f.Staleness > 0 {
		return math.Min(1.0, f.Staleness)
	}
	switch strings.ToLower(f.FreshnessState) {
	case "fresh":
		return 0.0
	case "modified":
		return 0.35
	case "stale":
		return 0.7
	case "invalid":
		return 1.0
	}
	if f.ModTime.IsZero() {
		return 0.0
	}
	daysSince := time.Since(f.ModTime).Hours() / 24.0
	// Files not modified in 30+ days get a discount
	if daysSince > 30 {
		return math.Min(0.5, (daysSince-30)/90.0)
	}
	return 0.0
}

// IndexSymbols adds symbols to the index for a file.
func (r *Ranker) IndexSymbols(path string, symbols []string) {
	r.symbols.mu.Lock()
	defer r.symbols.mu.Unlock()

	// Clear existing entries for this path
	if old, ok := r.symbols.paths[path]; ok {
		for _, sym := range old {
			r.removeFromSlice(r.symbols.symbols, sym, path)
		}
	}

	r.symbols.paths[path] = symbols
	for _, sym := range symbols {
		r.symbols.symbols[sym] = append(r.symbols.symbols[sym], path)
	}
}

// IndexKeywords adds searchable keywords for a file.
func (r *Ranker) IndexKeywords(path string, keywords []string) {
	r.symbols.mu.Lock()
	defer r.symbols.mu.Unlock()

	for _, kw := range keywords {
		kwLower := strings.ToLower(kw)
		if !containsString(r.symbols.keywords[kwLower], path) {
			r.symbols.keywords[kwLower] = append(r.symbols.keywords[kwLower], path)
		}
	}
}

// FindByKeyword returns files matching a keyword.
func (r *Ranker) FindByKeyword(keyword string) []string {
	r.symbols.mu.RLock()
	defer r.symbols.mu.RUnlock()

	kwLower := strings.ToLower(keyword)
	return r.symbols.keywords[kwLower]
}

// FindBySymbol returns files containing a symbol.
func (r *Ranker) FindBySymbol(symbol string) []string {
	r.symbols.mu.RLock()
	defer r.symbols.mu.RUnlock()

	return r.symbols.symbols[symbol]
}

// ClearCache clears the score cache.
func (r *Ranker) ClearCache() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache = make(map[string]*scoreCacheEntry)
}

// Helper functions

func (r *Ranker) removeFromSlice(m map[string][]string, key, val string) {
	slice := m[key]
	for i, v := range slice {
		if v == val {
			m[key] = append(slice[:i], slice[i+1:]...)
			break
		}
	}
}

func extractKeywords(intent string) []string {
	// Remove common stop words and extract meaningful keywords
	stopWords := map[string]bool{
		"a": true, "an": true, "the": true, "and": true, "or": true,
		"is": true, "are": true, "was": true, "were": true, "be": true,
		"been": true, "being": true, "have": true, "has": true, "had": true,
		"do": true, "does": true, "did": true, "will": true, "would": true,
		"could": true, "should": true, "may": true, "might": true, "must": true,
		"to": true, "of": true, "in": true, "for": true, "on": true,
		"with": true, "at": true, "by": true, "from": true, "as": true,
		"into": true, "through": true, "during": true, "before": true,
		"after": true, "above": true, "below": true, "between": true,
		"under": true, "again": true, "further": true, "then": true,
		"once": true, "here": true, "there": true, "when": true, "where": true,
		"why": true, "how": true, "all": true, "each": true, "few": true,
		"more": true, "most": true, "other": true, "some": true, "such": true,
		"no": true, "nor": true, "not": true, "only": true, "own": true,
		"same": true, "so": true, "than": true, "too": true, "very": true,
		"can": true, "just": true, "now": true, "this": true, "that": true,
		"it": true, "its": true, "i": true, "we": true, "you": true, "he": true,
		"she": true, "they": true, "me": true, "us": true, "him": true,
		"her": true, "them": true, "my": true, "our": true, "your": true,
		"his": true, "their": true,
	}

	words := strings.Fields(strings.ToLower(intent))
	var keywords []string
	seen := make(map[string]bool)

	for _, word := range words {
		// Clean punctuation
		word = strings.Trim(word, ".,!?:;\"'()[]{}/-")
		if len(word) < 2 || stopWords[word] || seen[word] {
			continue
		}
		seen[word] = true
		keywords = append(keywords, word)
	}

	return keywords
}

// fuzzyMatch performs fuzzy string matching.
func fuzzyMatch(s, pattern string) bool {
	s = strings.ToLower(s)
	pattern = strings.ToLower(pattern)
	if strings.Contains(s, pattern) {
		return true
	}
	// Simple prefix/suffix matching
	if strings.HasPrefix(s, pattern) || strings.HasSuffix(s, pattern) {
		return true
	}
	// Levenshtein distance for short strings
	if len(pattern) <= 8 && len(s) <= 20 {
		return levenshteinDistance(s, pattern) <= 2
	}
	return false
}

// levenshteinDistance calculates the edit distance between two strings.
func levenshteinDistance(s, t string) int {
	if len(s) == 0 {
		return utf8.RuneCountInString(t)
	}
	if len(t) == 0 {
		return utf8.RuneCountInString(s)
	}

	sRunes := []rune(s)
	tRunes := []rune(t)
	sLen := len(sRunes)
	tLen := len(tRunes)

	// Use single row optimization
	prev := make([]int, tLen+1)
	curr := make([]int, tLen+1)

	for j := 0; j <= tLen; j++ {
		prev[j] = j
	}

	for i := 1; i <= sLen; i++ {
		curr[0] = i
		for j := 1; j <= tLen; j++ {
			cost := 1
			if sRunes[i-1] == tRunes[j-1] {
				cost = 0
			}
			curr[j] = min(min(curr[j-1]+1, prev[j]+1), prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}

	return prev[tLen]
}

func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// ExtractPathKeywords extracts relevant keywords from a file path.
func ExtractPathKeywords(path string) []string {
	// Get base name and directory components
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)

	var keywords []string

	// Split name by common separators
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '_' || r == '-' || r == '.'
	})
	keywords = append(keywords, parts...)

	// Add camelCase splits
	keywords = append(keywords, splitCamelCase(name)...)

	// Add directory components, splitting each component as needed.
	dirParts := strings.Split(dir, string(filepath.Separator))
	for _, dp := range dirParts {
		if dp != "" && dp != "." {
			keywords = append(keywords, strings.FieldsFunc(dp, func(r rune) bool {
				return r == '_' || r == '-' || r == '.'
			})...)
			keywords = append(keywords, splitCamelCase(dp)...)
		}
	}

	// Add file extension as keyword (without dot)
	if ext != "" {
		keywords = append(keywords, strings.TrimPrefix(ext, "."))
	}

	return keywords
}

// splitCamelCase splits a camelCase or PascalCase string into words.
func splitCamelCase(s string) []string {
	var words []string
	var current strings.Builder

	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			if current.Len() > 0 {
				words = append(words, strings.ToLower(current.String()))
				current.Reset()
			}
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		words = append(words, strings.ToLower(current.String()))
	}

	return words
}
