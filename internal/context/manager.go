package context

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Manager is the central coordinator for context management.
type Manager struct {
	mu sync.RWMutex

	// Core components
	ranker   *Ranker
	budget   *TokenBudget
	detector *StalenessDetector
	graph    *DependencyGraph
	analyzer *DependencyAnalyzer
	selector *ContextSelector

	// Caches
	fileCache   map[string]*cachedFile
	accessLog   []accessEntry
	accessLimit int

	// Configuration
	rootDir string
	config  ManagerConfig
}

type cachedFile struct {
	path      string
	content   string
	tokens    int
	symbols   []string
	keywords  []string
	fetchedAt time.Time
}

type accessEntry struct {
	path      string
	timestamp time.Time
}

// ManagerConfig configures the context manager.
type ManagerConfig struct {
	ModelLimits     ModelTokenLimits
	BudgetConfig    TokenBudgetConfig
	RankingConfig   RankingConfig
	StalenessConfig StalenessConfig
	MaxCachedFiles  int
	AccessLogLimit  int
}

// DefaultManagerConfig returns sensible defaults.
func DefaultManagerConfig() ManagerConfig {
	return ManagerConfig{
		ModelLimits:     ModelLimitsGeminiFlash,
		BudgetConfig:    DefaultBudgetConfig(),
		RankingConfig:   DefaultRankingConfig(),
		StalenessConfig: DefaultStalenessConfig(),
		MaxCachedFiles:  100,
		AccessLogLimit:  1000,
	}
}

// NewManager creates a new context manager.
func NewManager(rootDir string, cfg ManagerConfig) *Manager {
	budget := NewTokenBudget(cfg.ModelLimits, cfg.BudgetConfig)
	ranker := NewRanker(cfg.RankingConfig)
	detector := NewStalenessDetector(rootDir, cfg.StalenessConfig)
	graph := NewDependencyGraph()
	analyzer := NewDependencyAnalyzer(rootDir)

	// Register default parsers
	analyzer.RegisterParser(&GoImportParser{})
	analyzer.RegisterParser(&TypeScriptImportParser{})
	analyzer.RegisterParser(&PythonImportParser{})

	selector := NewContextSelector(graph, ranker, budget, detector)

	return &Manager{
		ranker:      ranker,
		budget:      budget,
		detector:    detector,
		graph:       graph,
		analyzer:    analyzer,
		selector:    selector,
		fileCache:   make(map[string]*cachedFile),
		accessLog:   make([]accessEntry, 0, cfg.AccessLogLimit),
		accessLimit: cfg.AccessLogLimit,
		rootDir:     rootDir,
		config:      cfg,
	}
}

// GetContext retrieves context for a request.
func (m *Manager) GetContext(ctx context.Context, req ContextRequest) (*ContextResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	startTime := time.Now()

	// Record access
	for _, f := range req.ActiveFiles {
		m.recordAccess(f)
	}

	// Analyze dependencies if needed (collect errors)
	var analysisErrs []string
	for _, f := range req.ActiveFiles {
		if err := m.analyzer.AnalyzeFile(ctx, f); err != nil {
			analysisErrs = append(analysisErrs, fmt.Sprintf("%s: %v", f, err))
		}
	}

	// Select context files
	available := m.budget.AvailableForContext()
	if req.TokenBudget > 0 && req.TokenBudget < available {
		available = req.TokenBudget
	}

	items, signals, err := m.selector.SelectContext(ctx, req.Intent, req.ActiveFiles, available, req.IncludeDeps)
	if err != nil {
		return nil, err
	}

	// Build response
	resp := &ContextResponse{
		Items:       items,
		Signals:     signals,
		SelectionMs: time.Since(startTime).Milliseconds(),
		TotalTokens: sumTokens(items),
		BudgetUsed:  sumTokens(items),
		BudgetTotal: available,
	}

	// Update budget usage
	m.budget.UseContextFiles(resp.TotalTokens)

	// If any analysis errors occurred, return response with aggregated error
	if len(analysisErrs) > 0 {
		return resp, fmt.Errorf("analysis errors: %s", strings.Join(analysisErrs, "; "))
	}

	return resp, nil
}

// ContextRequest describes a context retrieval request.
type ContextRequest struct {
	Intent      string   // User's intent/query
	ActiveFiles []string // Currently active/open files
	TokenBudget int      // Maximum tokens to use (0 = use default)
	IncludeDeps bool     // Include dependency files
}

// ContextResponse contains selected context.
type ContextResponse struct {
	Items       []ContextItem
	Signals     []ContextSignal
	SelectionMs int64
	TotalTokens int
	BudgetUsed  int
	BudgetTotal int
}

// GetFileContext retrieves or caches a file's context.
func (m *Manager) GetFileContext(ctx context.Context, path string) (*FileContext, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check cache
	if cached, exists := m.fileCache[path]; exists {
		// Check staleness
		status, err := m.detector.Check(ctx, path)
		if err == nil && !status.NeedsRefresh {
			m.recordAccess(path)
			return &FileContext{
				Path:    cached.path,
				Content: cached.content,
				Symbols: cached.symbols,
				ModTime: status.ModTime,
			}, nil
		}
	}

	// Load file
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	info, _ := os.Stat(path)

	// Extract keywords and symbols
	keywords := ExtractPathKeywords(path)
	symbols := extractSymbols(string(content), filepath.Ext(path))

	// Cache
	cached := &cachedFile{
		path:      path,
		content:   string(content),
		tokens:    EstimateTokens(string(content)),
		symbols:   symbols,
		keywords:  keywords,
		fetchedAt: time.Now(),
	}
	m.fileCache[path] = cached

	// Index for search
	m.ranker.IndexSymbols(path, symbols)
	m.ranker.IndexKeywords(path, keywords)

	// Evict old entries if needed
	m.evictCache()

	m.recordAccess(path)
	m.detector.MarkFresh(path)

	return &FileContext{
		Path:    path,
		Content: string(content),
		Symbols: symbols,
		ModTime: info.ModTime(),
	}, nil
}

// RankFiles ranks files by relevance to an intent.
func (m *Manager) RankFiles(ctx context.Context, paths []string, intent string, limit int) ([]RelevanceScore, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Build file contexts
	var fileContexts []FileContext
	for _, path := range paths {
		fc, err := m.buildFileContext(ctx, path)
		if err != nil {
			continue
		}
		fileContexts = append(fileContexts, *fc)
	}

	return m.ranker.RankFiles(fileContexts, intent, limit), nil
}

// buildFileContext builds a FileContext from cache or disk.
func (m *Manager) buildFileContext(ctx context.Context, path string) (*FileContext, error) {
	// Check cache first
	if cached, exists := m.fileCache[path]; exists {
		accessCount := m.getAccessCount(path)
		lastAccess := m.getLastAccess(path)

		return &FileContext{
			Path:           cached.path,
			Content:        cached.content,
			Symbols:        cached.symbols,
			AccessCount:    accessCount,
			LastAccessTime: lastAccess,
			Dependencies:   m.graph.GetDependencies(path),
			Dependents:     m.graph.GetDependents(path),
		}, nil
	}

	// Load from disk
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	info, _ := os.Stat(path)
	symbols := extractSymbols(string(content), filepath.Ext(path))

	return &FileContext{
		Path:         path,
		Content:      string(content),
		Symbols:      symbols,
		ModTime:      info.ModTime(),
		Dependencies: m.graph.GetDependencies(path),
		Dependents:   m.graph.GetDependents(path),
	}, nil
}

// GetBudgetSummary returns current token budget usage.
func (m *Manager) GetBudgetSummary() BudgetSummary {
	return m.budget.Summary()
}

// GetStalenessReport returns staleness information for all cached files.
func (m *Manager) GetStalenessReport(ctx context.Context) (map[string]*FileStatus, error) {
	m.mu.RLock()
	paths := make([]string, 0, len(m.fileCache))
	for path := range m.fileCache {
		paths = append(paths, path)
	}
	m.mu.RUnlock()

	return m.detector.CheckBatch(ctx, paths)
}

// InvalidateFile marks a file as needing refresh.
func (m *Manager) InvalidateFile(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.detector.Invalidate(path)
	delete(m.fileCache, path)
}

// RefreshStale refreshes all stale files.
func (m *Manager) RefreshStale(ctx context.Context) error {
	stale := m.detector.GetStaleFiles()

	for _, path := range stale {
		_, err := m.GetFileContext(ctx, path)
		if err != nil {
			continue // Skip errors
		}
	}

	return nil
}

// AnalyzeDependencies analyzes dependencies for a directory.
func (m *Manager) AnalyzeDependencies(ctx context.Context, dir string) error {
	return m.analyzer.AnalyzeDirectory(ctx, dir)
}

// GetDependencies returns dependencies for a file.
func (m *Manager) GetDependencies(path string) []string {
	return m.graph.GetDependencies(path)
}

// GetDependents returns files that depend on a file.
func (m *Manager) GetDependents(path string) []string {
	return m.graph.GetDependents(path)
}

// FindByKeyword finds files matching a keyword.
func (m *Manager) FindByKeyword(keyword string) []string {
	return m.ranker.FindByKeyword(keyword)
}

// FindBySymbol finds files containing a symbol.
func (m *Manager) FindBySymbol(symbol string) []string {
	return m.ranker.FindBySymbol(symbol)
}

// RecentFiles returns recently accessed files.
func (m *Manager) RecentFiles(limit int) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Get unique recent files
	seen := make(map[string]bool)
	var result []string

	for i := len(m.accessLog) - 1; i >= 0 && len(result) < limit; i-- {
		entry := m.accessLog[i]
		if !seen[entry.path] {
			seen[entry.path] = true
			result = append(result, entry.path)
		}
	}

	return result
}

// FrequentFiles returns frequently accessed files.
func (m *Manager) FrequentFiles(limit int) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Count access frequency
	counts := make(map[string]int)
	for _, entry := range m.accessLog {
		counts[entry.path]++
	}

	// Sort by frequency
	type fileCount struct {
		path  string
		count int
	}
	var files []fileCount
	for path, count := range counts {
		files = append(files, fileCount{path, count})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].count > files[j].count
	})

	// Return top N
	result := make([]string, 0, limit)
	for i := 0; i < len(files) && i < limit; i++ {
		result = append(result, files[i].path)
	}

	return result
}

// SetModel updates the model and reallocates budget.
func (m *Manager) SetModel(modelLimits ModelTokenLimits) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.config.ModelLimits = modelLimits
	m.budget = NewTokenBudget(modelLimits, m.config.BudgetConfig)
}

// Clear clears all caches and state.
func (m *Manager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.fileCache = make(map[string]*cachedFile)
	m.accessLog = m.accessLog[:0]
	m.detector.Clear()
	m.ranker.ClearCache()
}

// recordAccess records a file access.
func (m *Manager) recordAccess(path string) {
	m.accessLog = append(m.accessLog, accessEntry{
		path:      path,
		timestamp: time.Now(),
	})

	// Trim if over limit and copy to release backing array to avoid memory leak
	if m.accessLimit > 0 && len(m.accessLog) > m.accessLimit {
		sz := m.accessLimit
		start := len(m.accessLog) - sz
		newLog := make([]accessEntry, sz)
		copy(newLog, m.accessLog[start:])
		m.accessLog = newLog
	}
}

// getAccessCount returns the access count for a file.
func (m *Manager) getAccessCount(path string) int {
	count := 0
	for _, entry := range m.accessLog {
		if entry.path == path {
			count++
		}
	}
	return count
}

// getLastAccess returns the last access time for a file.
func (m *Manager) getLastAccess(path string) time.Time {
	for i := len(m.accessLog) - 1; i >= 0; i-- {
		if m.accessLog[i].path == path {
			return m.accessLog[i].timestamp
		}
	}
	return time.Time{}
}

// evictCache evicts old cache entries if over limit.
func (m *Manager) evictCache() {
	if len(m.fileCache) <= m.config.MaxCachedFiles {
		return
	}

	// Sort by access recency
	type cacheEntry struct {
		path      string
		fetchedAt time.Time
	}
	var entries []cacheEntry
	for path, cached := range m.fileCache {
		entries = append(entries, cacheEntry{path, cached.fetchedAt})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].fetchedAt.Before(entries[j].fetchedAt)
	})

	// Evict oldest entries
	toEvict := len(m.fileCache) - m.config.MaxCachedFiles
	for i := 0; i < toEvict; i++ {
		delete(m.fileCache, entries[i].path)
	}
}

// extractSymbols extracts symbol names from code.
func extractSymbols(content, ext string) []string {
	var symbols []string

	switch ext {
	case ".go":
		symbols = extractGoSymbols(content)
	case ".ts", ".tsx", ".js", ".jsx":
		symbols = extractTSSymbols(content)
	case ".py":
		symbols = extractPySymbols(content)
	}

	return symbols
}

func extractGoSymbols(content string) []string {
	var symbols []string
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Function declarations
		if strings.HasPrefix(line, "func ") {
			if idx := strings.Index(line, "("); idx != -1 {
				name := strings.TrimPrefix(line[:idx], "func ")
				name = strings.TrimSpace(name)
				// Handle receiver methods
				if strings.HasPrefix(name, "(") {
					if closeIdx := strings.Index(name, ")"); closeIdx != -1 {
						name = strings.TrimSpace(name[closeIdx+1:])
					}
				}
				if name != "" && isExported(name) {
					symbols = append(symbols, name)
				}
			}
		}

		// Type declarations
		if strings.HasPrefix(line, "type ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 && isExported(parts[1]) {
				symbols = append(symbols, parts[1])
			}
		}

		// Const/var declarations
		if strings.HasPrefix(line, "const ") || strings.HasPrefix(line, "var ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				name := strings.TrimSuffix(parts[1], "=")
				if isExported(name) {
					symbols = append(symbols, name)
				}
			}
		}
	}

	return symbols
}

func extractTSSymbols(content string) []string {
	var symbols []string
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Export function/class/const/interface
		if strings.HasPrefix(line, "export ") {
			rest := strings.TrimPrefix(line, "export ")
			if strings.HasPrefix(rest, "default ") {
				rest = strings.TrimPrefix(rest, "default ")
			}

			var keyword string
			for _, kw := range []string{"function ", "class ", "const ", "let ", "var ", "interface ", "type ", "enum "} {
				if strings.HasPrefix(rest, kw) {
					keyword = kw
					break
				}
			}

			if keyword != "" {
				rest = strings.TrimPrefix(rest, keyword)
				parts := strings.FieldsFunc(rest, func(r rune) bool {
					return r == ' ' || r == '(' || r == '<' || r == ':' || r == '='
				})
				if len(parts) > 0 {
					symbols = append(symbols, parts[0])
				}
			}
		}
	}

	return symbols
}

func extractPySymbols(content string) []string {
	var symbols []string
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Class definitions
		if strings.HasPrefix(trimmed, "class ") {
			rest := strings.TrimPrefix(trimmed, "class ")
			parts := strings.FieldsFunc(rest, func(r rune) bool {
				return r == '(' || r == ':'
			})
			if len(parts) > 0 && !strings.HasPrefix(parts[0], "_") {
				symbols = append(symbols, parts[0])
			}
		}

		// Function definitions (top-level)
		if strings.HasPrefix(line, "def ") {
			rest := strings.TrimPrefix(line, "def ")
			parts := strings.FieldsFunc(rest, func(r rune) bool {
				return r == '('
			})
			if len(parts) > 0 && !strings.HasPrefix(parts[0], "_") {
				symbols = append(symbols, parts[0])
			}
		}
	}

	return symbols
}

func isExported(name string) bool {
	if len(name) == 0 {
		return false
	}
	r := rune(name[0])
	return r >= 'A' && r <= 'Z'
}

func sumTokens(items []ContextItem) int {
	// Use int64 accumulator to avoid integer overflow on 32-bit systems
	var total64 int64
	for _, item := range items {
		total64 += int64(item.Tokens)
	}
	if total64 > int64(int(^uint(0)>>1)) { // cap at max int
		return int(^uint(0) >> 1)
	}
	return int(total64)
}
