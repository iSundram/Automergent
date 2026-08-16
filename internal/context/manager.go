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
	ranker     *Ranker
	budget     *TokenBudget
	detector   *StalenessDetector
	graph      *DependencyGraph
	analyzer   *DependencyAnalyzer
	selector   *ContextSelector
	summarizer *ContextSummarizer

	// Transcript (durable conversation history)
	transcript *TranscriptManager

	// Adaptive token estimation
	adaptiveCalc *AdaptiveTokenCalculator

	// Telemetry
	telemetry *TelemetryCollector

	// Caches
	fileCache   map[string]*cachedFile
	accessLog   []accessEntry
	accessLimit int

	// Analysis memoization
	analysisCache map[string]analysisCacheEntry
	analysisMu    sync.Mutex
	inFlight      map[string]chan error

	// Configuration
	rootDir string
	config  ManagerConfig
}

type analysisCacheEntry struct {
	modTime time.Time
	err     error
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

	// Create transcript with persistence
	transcriptPath := TranscriptPathFor(rootDir, "default")
	transcript := NewTranscriptManager(NewTranscript(transcriptPath))

	// Create adaptive token calculator with persistence
	adaptiveCalc := NewAdaptiveTokenCalculator(cfg.ModelLimits.Name).WithPersistence(rootDir)

	// Create telemetry collector with persistence
	telemetry := NewTelemetryCollector(rootDir, 1000)
	_ = telemetry.Load()

	m := &Manager{
		ranker:        ranker,
		budget:        budget,
		detector:      detector,
		graph:         graph,
		analyzer:      analyzer,
		selector:      selector,
		summarizer:    NewContextSummarizer(nil),
		transcript:    transcript,
		adaptiveCalc:  adaptiveCalc,
		telemetry:     telemetry,
		fileCache:     make(map[string]*cachedFile),
		accessLog:     make([]accessEntry, 0, cfg.AccessLogLimit),
		accessLimit:   cfg.AccessLogLimit,
		rootDir:       rootDir,
		config:        cfg,
		analysisCache: make(map[string]analysisCacheEntry),
		inFlight:      make(map[string]chan error),
	}

	// Set file provider for selector to get symbols/access info
	m.selector.SetFileProvider(func(ctx context.Context, path string) (string, bool) {
		fc, err := m.GetFileContext(ctx, path)
		if err != nil {
			return "", false
		}
		return fc.Content, true
	})

	return m
}

// GetContext retrieves context for a request.
func (m *Manager) GetContext(ctx context.Context, req ContextRequest) (*ContextResponse, error) {
	startTime := time.Now()

	// Record access and snapshot active files under lock
	m.mu.RLock()
	for _, f := range req.ActiveFiles {
		m.recordAccessUnlocked(f)
	}
	activeFiles := make([]string, len(req.ActiveFiles))
	copy(activeFiles, req.ActiveFiles)
	m.mu.RUnlock()

	// Analyze dependencies with memoization and in-flight dedup (no global lock held)
	var analysisErrs []string
	for _, f := range activeFiles {
		if err := m.analyzeFileWithMemo(ctx, f); err != nil {
			analysisErrs = append(analysisErrs, fmt.Sprintf("%s: %v", f, err))
		}
	}

	// Select context files
	available := m.budget.AvailableForContext()
	if req.TokenBudget > 0 && req.TokenBudget < available {
		available = req.TokenBudget
	}

	items, signals, err := m.selector.SelectContext(ctx, req.Intent, activeFiles, available, req.IncludeDeps)
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
		Warns:       analysisErrs,
	}

	// Update budget usage
	m.budget.UseContextFiles(resp.TotalTokens)

	// Return nil error when items exist (only error for hard failures with nil resp)
	if len(items) > 0 {
		return resp, nil
	}
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
	Warns       []string
	Errors      []string
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
			m.recordAccessUnlocked(path)
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

	// Caller holds m.mu (GetFileContext locks at entry)
	m.recordAccessUnlocked(path)
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

	// Update adaptive calculator model
	if m.adaptiveCalc != nil {
		// Create new calculator for the new model
		m.adaptiveCalc = NewAdaptiveTokenCalculator(modelLimits.Name).WithPersistence(m.rootDir)
	}
}

// TranscriptManager returns the transcript manager for durable conversation history.
func (m *Manager) TranscriptManager() *TranscriptManager {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.transcript
}

// AdaptiveCalculator returns the adaptive token calculator.
func (m *Manager) AdaptiveCalculator() *AdaptiveTokenCalculator {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.adaptiveCalc
}

// Telemetry returns the telemetry collector.
func (m *Manager) Telemetry() *TelemetryCollector {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.telemetry
}

// RecordGroundTruth records actual token usage from the provider for learning.
func (m *Manager) RecordGroundTruth(promptText string, actualTokens int) {
	if m.adaptiveCalc != nil {
		m.adaptiveCalc.RecordGroundTruth(promptText, actualTokens)
	}
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
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordAccessUnlocked(path)
}

// recordAccessUnlocked records a file access (caller must hold lock).
func (m *Manager) recordAccessUnlocked(path string) {
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

// analyzeFileWithMemo analyzes a file with memoization and in-flight deduplication.
func (m *Manager) analyzeFileWithMemo(ctx context.Context, path string) error {
	// Check cache first (with modtime validation)
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	modTime := info.ModTime()

	m.analysisMu.Lock()
	if entry, ok := m.analysisCache[path]; ok && entry.modTime.Equal(modTime) {
		err := entry.err
		m.analysisMu.Unlock()
		return err
	}

	// Check if analysis is already in flight
	if ch, ok := m.inFlight[path]; ok {
		m.analysisMu.Unlock()
		select {
		case err := <-ch:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// Start new analysis
	ch := make(chan error, 1)
	m.inFlight[path] = ch
	m.analysisMu.Unlock()

	// Perform analysis
	err = m.analyzer.AnalyzeFile(ctx, path)

	// Update cache and notify waiters
	m.analysisMu.Lock()
	m.analysisCache[path] = analysisCacheEntry{modTime: modTime, err: err}
	delete(m.inFlight, path)
	m.analysisMu.Unlock()

	ch <- err
	close(ch)

	return err
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

// evictCache evicts old cache entries if over limit using LRU based on access log.
func (m *Manager) evictCache() {
	if len(m.fileCache) <= m.config.MaxCachedFiles {
		return
	}

	// Build last access time map from access log (most recent first)
	lastAccess := make(map[string]time.Time)
	for i := len(m.accessLog) - 1; i >= 0; i-- {
		entry := m.accessLog[i]
		if _, exists := lastAccess[entry.path]; !exists {
			lastAccess[entry.path] = entry.timestamp
		}
	}

	// Sort by last access time (oldest first for eviction)
	type cacheEntry struct {
		path       string
		lastAccess time.Time
	}
	var entries []cacheEntry
	for path := range m.fileCache {
		la := lastAccess[path]
		if la.IsZero() {
			// Never accessed, use fetchedAt as fallback
			if cached, ok := m.fileCache[path]; ok {
				la = cached.fetchedAt
			}
		}
		entries = append(entries, cacheEntry{path, la})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].lastAccess.Before(entries[j].lastAccess)
	})

	// Evict oldest entries (least recently used)
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

// --- Context State Management ---

// IgnoreContext moves a context item to ignored state with a summary.
func (m *Manager) IgnoreContext(path, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cached, exists := m.fileCache[path]
	if !exists {
		return
	}

	item := ContextItem{
		Path:    cached.path,
		Content: cached.content,
		Tokens:  cached.tokens,
	}

	now := time.Now()
	item.State = ContextIgnored
	item.IgnoreReason = reason
	item.IgnoredAt = &now
	item.Summary = m.summarizer.SummarizeIgnored(item)
}

// ResumeContext re-activates an ignored context item.
func (m *Manager) ResumeContext(path string) *ContextItem {
	m.mu.Lock()
	defer m.mu.Unlock()

	cached, exists := m.fileCache[path]
	if !exists {
		return nil
	}

	now := time.Now()
	return &ContextItem{
		Path:       cached.path,
		Content:    cached.content,
		Tokens:     cached.tokens,
		State:      ContextResumed,
		ResumedAt:  &now,
	}
}

// GetIgnoredSummaries returns summaries of all ignored context items.
func (m *Manager) GetIgnoredSummaries() []ContextItem {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []ContextItem
	for _, cached := range m.fileCache {
		item := ContextItem{
			Path:         cached.path,
			Content:      cached.content,
			Tokens:       cached.tokens,
			State:        ContextIgnored,
			Summary:      m.summarizer.SummarizeIgnored(ContextItem{Content: cached.content}),
			PriorityLevel: PriorityLow,
		}
		result = append(result, item)
	}
	return result
}

// SetSummarizer sets a custom summarizer for context summarization.
func (m *Manager) SetSummarizer(s *ContextSummarizer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.summarizer = s
}

// --- Memory Integration ---

// GetContextWithMemory retrieves context enriched with relevant memory entries.
func (m *Manager) GetContextWithMemory(ctx context.Context, req ContextRequest, memory *Memory) (*ContextResponse, error) {
	resp, err := m.GetContext(ctx, req)
	if err != nil {
		return resp, err
	}

	if memory == nil {
		return resp, nil
	}

	entries := memory.RelevantTo(req.Intent)

	var memItems []ContextItem
	for _, entry := range entries {
		tokens := EstimateTokens(entry.Value)
		memItems = append(memItems, ContextItem{
			Path:         "memory:" + entry.Key,
			Content:      entry.Value,
			Tokens:       tokens,
			Priority:     1.0,
			Required:     true,
			PriorityLevel: PriorityCritical,
		})
	}

	allItems := make([]ContextItem, 0, len(memItems)+len(resp.Items))
	allItems = append(allItems, memItems...)
	allItems = append(allItems, resp.Items...)
	resp.Items = allItems
	resp.TotalTokens = sumTokens(allItems)

	return resp, nil
}

// --- Personalized Context ---

// PersonalizedContext is a tailored context bundle for a specific agent role.
type PersonalizedContext struct {
	Role        string        `json:"role"`
	Intent      string        `json:"intent"`
	Items       []ContextItem `json:"items"`
	TotalTokens int           `json:"total_tokens"`
	ToolFilter  []string      `json:"tool_filter"`
}

// BuildPersonalizedContext creates a tailored context bundle for a specific agent role.
func (m *Manager) BuildPersonalizedContext(
	role string,
	intent string,
	activeFiles []string,
	toolFilter []string,
	tokenBudget int,
) (*PersonalizedContext, error) {
	req := ContextRequest{
		Intent:      intent,
		ActiveFiles: activeFiles,
		TokenBudget: tokenBudget,
		IncludeDeps: true,
	}
	resp, err := m.GetContext(context.Background(), req)
	if err != nil {
		return nil, err
	}

	filtered := m.filterContextByRole(resp.Items, role, toolFilter)

	return &PersonalizedContext{
		Role:        role,
		Intent:      intent,
		Items:       filtered,
		TotalTokens: sumTokens(filtered),
		ToolFilter:  toolFilter,
	}, nil
}

// filterContextByRole filters context items based on role and tool requirements.
func (m *Manager) filterContextByRole(items []ContextItem, role string, toolFilter []string) []ContextItem {
	if len(toolFilter) == 0 {
		return items
	}

	neededPatterns := make(map[string]bool)
	for _, tool := range toolFilter {
		for _, pattern := range toolContextPatterns(tool) {
			neededPatterns[pattern] = true
		}
	}

	var filtered []ContextItem
	for _, item := range items {
		if item.PriorityLevel == PriorityCritical || item.Required {
			filtered = append(filtered, item)
			continue
		}
		if matchesAnyPattern(item.Path, neededPatterns) {
			filtered = append(filtered, item)
		}
	}

	return filtered
}

// --- Tool-Specific Context Injection ---

// SelectContextForTools selects only context relevant to the given tools.
func (m *Manager) SelectContextForTools(
	tools []string,
	intent string,
	activeFiles []string,
	tokenBudget int,
) ([]ContextItem, error) {
	req := ContextRequest{
		Intent:      intent,
		ActiveFiles: activeFiles,
		TokenBudget: tokenBudget,
		IncludeDeps: true,
	}
	resp, err := m.GetContext(context.Background(), req)
	if err != nil {
		return nil, err
	}

	neededPatterns := make(map[string]bool)
	for _, tool := range tools {
		for _, pattern := range toolContextPatterns(tool) {
			neededPatterns[pattern] = true
		}
	}

	var filtered []ContextItem
	for _, item := range resp.Items {
		if item.Required || item.PriorityLevel == PriorityCritical {
			filtered = append(filtered, item)
			continue
		}
		if matchesAnyPattern(item.Path, neededPatterns) {
			filtered = append(filtered, item)
		}
	}

	return filtered, nil
}

// toolContextPatterns returns the context patterns needed by a tool.
func toolContextPatterns(tool string) []string {
	switch strings.ToLower(tool) {
	case "glob":
		return []string{".go", ".ts", ".js", ".py", ".tsx", ".jsx", ".md", ".yaml", ".yml", ".json"}
	case "grep", "search":
		return []string{".go", ".ts", ".js", ".py", ".tsx", ".jsx"}
	case "view", "read":
		return []string{".go", ".ts", ".js", ".py", ".tsx", ".jsx", ".md"}
	case "edit", "write":
		return []string{".go", ".ts", ".js", ".py", ".tsx", ".jsx"}
	case "list_directory":
		return []string{"internal", "cmd", "pkg", "src"}
	default:
		return []string{".go", ".ts", ".js", ".py"}
	}
}

// matchesAnyPattern checks if a file path matches any of the needed patterns.
func matchesAnyPattern(path string, patterns map[string]bool) bool {
	pathLower := strings.ToLower(path)
	for pattern := range patterns {
		// Check if pattern is a file extension
		if strings.HasPrefix(pattern, ".") {
			if strings.HasSuffix(pathLower, pattern) {
				return true
			}
		}
		// Check if pattern is a directory name
		if strings.Contains(pathLower, "/"+pattern+"/") || strings.Contains(pathLower, pattern+"/") {
			return true
		}
		// Check if pattern appears anywhere in path
		if strings.Contains(pathLower, pattern) {
			return true
		}
	}
	return len(patterns) == 0
}
