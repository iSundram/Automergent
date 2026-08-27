// Package workspace provides workspace-wide diagnostics aggregation.
package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/iSundram/Automergent/internal/diagnostics"
	"github.com/iSundram/Automergent/internal/diagnostics/cache"
	"github.com/iSundram/Automergent/internal/diagnostics/types"
)

// WorkspaceDiagnostics aggregates diagnostics across a project.
type WorkspaceDiagnostics struct {
	rootDir       string
	filePatterns  []string
	excludePatterns []string
	maxWorkers    int

	mu           sync.RWMutex
	diagnostics  map[string][]types.Diagnostic // filePath -> diagnostics
	fileHashes   map[string]string             // filePath -> content hash
	lastScan     time.Time
	scanCount    int64
	totalFiles   int64
	totalDiags   int64

	cache        *cache.PersistentCache
	scanCancel   context.CancelFunc
	scanComplete chan struct{}
	progressFn   func(ProgressUpdate)

	// Watch for file changes
	watcher      *FileWatcher
	watcherCancel context.CancelFunc
}

// ProgressUpdate represents scan progress.
type ProgressUpdate struct {
	Phase        string  // "scanning", "parsing", "linting", "complete"
	FilesScanned int64
	TotalFiles   int64
	CurrentFile  string
	Diagnostics  int64
	Error        error
}

// FileWatcher watches for file changes.
type FileWatcher struct {
	rootDir      string
	patterns     []string
	exclude      []string
	changeFn     func(string) // filePath
	stopCh       chan struct{}
}

// NewWorkspaceDiagnostics creates a new workspace diagnostics aggregator.
func NewWorkspaceDiagnostics(rootDir string, options ...WorkspaceOption) *WorkspaceDiagnostics {
	wd := &WorkspaceDiagnostics{
		rootDir:         rootDir,
		filePatterns:    []string{"**/*.go", "**/*.py", "**/*.js", "**/*.ts", "**/*.rs", "**/*.java", "**/*.json", "**/*.yaml", "**/*.yml", "**/*.toml"},
		excludePatterns: []string{"**/vendor/**", "**/node_modules/**", "**/.git/**", "**/target/**", "**/build/**", "**/dist/**", "**/.automergent/**"},
		maxWorkers:      runtime.NumCPU(),
		diagnostics:     make(map[string][]types.Diagnostic),
		fileHashes:      make(map[string]string),
		scanComplete:    make(chan struct{}),
	}

	for _, opt := range options {
		opt(wd)
	}

	if wd.cache == nil {
		// Initialize default cache
		cacheDir := filepath.Join(rootDir, ".automergent", "diagnostics_cache")
		_ = cache.InitGlobalCache(cacheDir, 24*time.Hour)
		wd.cache = cache.GetGlobalCache()
	}

	// Initialize file watcher
	wd.watcher = NewFileWatcher(rootDir, wd.filePatterns, wd.excludePatterns, wd.onFileChanged)

	return wd
}

// RootDir returns the workspace root directory.
func (wd *WorkspaceDiagnostics) RootDir() string {
	return wd.rootDir
}

// WorkspaceOption configures WorkspaceDiagnostics.
type WorkspaceOption func(*WorkspaceDiagnostics)

// WithFilePatterns sets custom file patterns.
func WithFilePatterns(patterns []string) WorkspaceOption {
	return func(wd *WorkspaceDiagnostics) {
		wd.filePatterns = patterns
	}
}

// WithExcludePatterns sets custom exclude patterns.
func WithExcludePatterns(patterns []string) WorkspaceOption {
	return func(wd *WorkspaceDiagnostics) {
		wd.excludePatterns = patterns
	}
}

// WithMaxWorkers sets the number of worker goroutines.
func WithMaxWorkers(n int) WorkspaceOption {
	return func(wd *WorkspaceDiagnostics) {
		wd.maxWorkers = n
	}
}

// WithCache sets a custom cache.
func WithCache(c *cache.PersistentCache) WorkspaceOption {
	return func(wd *WorkspaceDiagnostics) {
		wd.cache = c
	}
}

// WithProgress sets a progress callback.
func WithProgress(fn func(ProgressUpdate)) WorkspaceOption {
	return func(wd *WorkspaceDiagnostics) {
		wd.progressFn = fn
	}
}

// emitProgress sends a progress update.
func (wd *WorkspaceDiagnostics) emitProgress(update ProgressUpdate) {
	if wd.progressFn != nil {
		wd.progressFn(update)
	}
}

// Start begins workspace scanning and file watching.
func (wd *WorkspaceDiagnostics) Start(ctx context.Context) error {
	wd.mu.Lock()
	if wd.scanCancel != nil {
		wd.mu.Unlock()
		return fmt.Errorf("already running")
	}
	scanCtx, cancel := context.WithCancel(ctx)
	wd.scanCancel = cancel
	wd.mu.Unlock()

	// Initial scan
	go wd.fullScan(scanCtx)

	// Start file watcher
	watcherCtx, watcherCancel := context.WithCancel(ctx)
	wd.watcherCancel = watcherCancel
	go wd.watcher.Start(watcherCtx, wd.onFileChanged)

	return nil
}

// Stop stops workspace scanning and file watching.
func (wd *WorkspaceDiagnostics) Stop() {
	wd.mu.Lock()
	defer wd.mu.Unlock()

	if wd.scanCancel != nil {
		wd.scanCancel()
		wd.scanCancel = nil
	}
	if wd.watcherCancel != nil {
		wd.watcherCancel()
		wd.watcherCancel = nil
	}
	if wd.watcher != nil {
		wd.watcher.Stop()
	}
}

// fullScan performs a complete workspace scan.
func (wd *WorkspaceDiagnostics) fullScan(ctx context.Context) error {
	defer close(wd.scanComplete)

	files, err := wd.findFiles()
	if err != nil {
		wd.emitProgress(ProgressUpdate{Phase: "error", Error: err})
		return err
	}

	wd.emitProgress(ProgressUpdate{
		Phase:      "scanning",
		TotalFiles: int64(len(files)),
	})

	sem := make(chan struct{}, wd.maxWorkers)
	var wg sync.WaitGroup
	var scanned int64

	for _, file := range files {
		select {
		case <-ctx.Done():
			wd.emitProgress(ProgressUpdate{Phase: "cancelled", Error: ctx.Err()})
			return ctx.Err()
		default:
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(f string) {
			defer wg.Done()
			defer func() { <-sem }()

			atomic.AddInt64(&scanned, 1)
			wd.emitProgress(ProgressUpdate{
				Phase:        "parsing",
				FilesScanned: scanned,
				TotalFiles:   int64(len(files)),
				CurrentFile:  f,
			})

			diags, err := wd.scanFile(ctx, f)
			if err != nil {
				// Log but continue
				return
			}

			wd.mu.Lock()
			wd.diagnostics[f] = diags
			wd.mu.Unlock()
		}(file)
	}

	wg.Wait()

	wd.mu.Lock()
	wd.lastScan = time.Now()
	wd.totalFiles = int64(len(files))
	wd.totalDiags = 0
	for _, d := range wd.diagnostics {
		wd.totalDiags += int64(len(d))
	}
	wd.scanCount++
	wd.mu.Unlock()

	wd.emitProgress(ProgressUpdate{
		Phase:      "complete",
		FilesScanned: int64(len(files)),
		TotalFiles: int64(len(files)),
		Diagnostics: wd.totalDiags,
	})
	return nil
}

// scanFile scans a single file using diagnostics with cache.
func (wd *WorkspaceDiagnostics) scanFile(ctx context.Context, filePath string) ([]types.Diagnostic, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	// Check cache first
	if wd.cache != nil {
		if cached, ok := wd.cache.Get(filePath, string(content), 24*time.Hour); ok {
			return cached, nil
		}
	}

	// Run diagnostics
	diags := diagnostics.Analyze(filePath, string(content))

	// Store in cache
	if wd.cache != nil && len(diags) > 0 {
		lang := diagnostics.DetectLanguage(filePath)
		wd.cache.Put(filePath, string(content), string(lang), diags, 24*time.Hour)
	}

	return diags, nil
}

// findFiles finds all matching files in the workspace.
func (wd *WorkspaceDiagnostics) findFiles() ([]string, error) {
	var files []string

	err := filepath.WalkDir(wd.rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err // Continue on error
		}

		if d.IsDir() {
			// Check exclude patterns for directories
			rel, _ := filepath.Rel(wd.rootDir, path)
			for _, pattern := range wd.excludePatterns {
				if matchPattern(rel, pattern) {
					return filepath.SkipDir
				}
			}
			return nil
		}

		// Check file patterns
		rel, _ := filepath.Rel(wd.rootDir, path)
		for _, pattern := range wd.filePatterns {
			if matchPattern(rel, pattern) {
				files = append(files, path)
				break
			}
		}
		return nil
	})

	return files, err
}

// matchPattern matches a path against a glob pattern.
func matchPattern(path, pattern string) bool {
	// Simple glob matching
	pattern = strings.ReplaceAll(pattern, "**/", "")
	pattern = strings.ReplaceAll(pattern, "*", ".*")
	pattern = "^" + pattern + "$"
	
	matched, _ := filepath.Match(strings.TrimPrefix(pattern, "^"), path)
	if matched {
		return true
	}
	// Try with full pattern
	matched, _ = filepath.Match(pattern, path)
	return matched
}

// onFileChanged handles file change events.
func (wd *WorkspaceDiagnostics) onFileChanged(filePath string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	diags, err := wd.scanFile(ctx, filePath)
	if err != nil {
		return
	}

	wd.mu.Lock()
	if len(diags) == 0 {
		delete(wd.diagnostics, filePath)
	} else {
		wd.diagnostics[filePath] = diags
	}
	wd.totalDiags = 0
	for _, d := range wd.diagnostics {
		wd.totalDiags += int64(len(d))
	}
	wd.mu.Unlock()

	wd.emitProgress(ProgressUpdate{
		Phase:       "updated",
		CurrentFile: filePath,
		Diagnostics: wd.totalDiags,
	})
}

// GetDiagnostics returns diagnostics for a specific file.
func (wd *WorkspaceDiagnostics) GetDiagnostics(filePath string) []types.Diagnostic {
	wd.mu.RLock()
	defer wd.mu.RUnlock()
	return wd.diagnostics[filePath]
}

// GetAllDiagnostics returns all diagnostics in the workspace.
func (wd *WorkspaceDiagnostics) GetAllDiagnostics() map[string][]types.Diagnostic {
	wd.mu.RLock()
	defer wd.mu.RUnlock()

	result := make(map[string][]types.Diagnostic, len(wd.diagnostics))
	for k, v := range wd.diagnostics {
		result[k] = v
	}
	return result
}

// GetDiagnosticsBySeverity returns diagnostics filtered by severity.
func (wd *WorkspaceDiagnostics) GetDiagnosticsBySeverity(severity string) map[string][]types.Diagnostic {
	wd.mu.RLock()
	defer wd.mu.RUnlock()

	result := make(map[string][]types.Diagnostic)
	for file, diags := range wd.diagnostics {
		var filtered []types.Diagnostic
		for _, d := range diags {
			if d.Severity == severity {
				filtered = append(filtered, d)
			}
		}
		if len(filtered) > 0 {
			result[file] = filtered
		}
	}
	return result
}

// GetStats returns workspace statistics.
func (wd *WorkspaceDiagnostics) GetStats() WorkspaceStats {
	wd.mu.RLock()
	defer wd.mu.RUnlock()

	bySeverity := make(map[string]int64)
	byLanguage := make(map[string]int64)

	for _, diags := range wd.diagnostics {
		for _, d := range diags {
			bySeverity[d.Severity]++
			byLanguage[d.Source]++
		}
	}

	return WorkspaceStats{
		TotalFiles:       wd.totalFiles,
		TotalDiagnostics: wd.totalDiags,
		BySeverity:       bySeverity,
		BySource:         byLanguage,
		LastScan:         wd.lastScan,
		ScanCount:        wd.scanCount,
	}
}

// WorkspaceStats holds workspace diagnostics statistics.
type WorkspaceStats struct {
	TotalFiles       int64             `json:"total_files"`
	TotalDiagnostics int64             `json:"total_diagnostics"`
	BySeverity       map[string]int64  `json:"by_severity"`
	BySource         map[string]int64  `json:"by_source"`
	LastScan         time.Time         `json:"last_scan"`
	ScanCount        int64             `json:"scan_count"`
}

// ReScan triggers a full rescan.
func (wd *WorkspaceDiagnostics) ReScan(ctx context.Context) error {
	wd.mu.Lock()
	wd.diagnostics = make(map[string][]types.Diagnostic)
	wd.fileHashes = make(map[string]string)
	wd.mu.Unlock()
	return wd.fullScan(ctx)
}

// ScanFile scans a specific file on demand.
func (wd *WorkspaceDiagnostics) ScanFile(ctx context.Context, filePath string) ([]types.Diagnostic, error) {
	return wd.scanFile(ctx, filePath)
}

// NewFileWatcher creates a new file watcher.
func NewFileWatcher(rootDir string, patterns, exclude []string, changeFn func(string)) *FileWatcher {
	return &FileWatcher{
		rootDir:  rootDir,
		patterns: patterns,
		exclude:  exclude,
		changeFn: changeFn,
		stopCh:   make(chan struct{}),
	}
}

// Start begins watching for file changes.
func (fw *FileWatcher) Start(ctx context.Context, changeFn func(string)) {
	fw.changeFn = changeFn
	// In a real implementation, this would use fsnotify
	// For now, we'll rely on periodic rescans
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		lastMod := make(map[string]time.Time)

		for {
			select {
			case <-ctx.Done():
				return
			case <-fw.stopCh:
				return
			case <-ticker.C:
				fw.checkChanges(lastMod)
			}
		}
	}()
}

func (fw *FileWatcher) checkChanges(lastMod map[string]time.Time) {
	filepath.WalkDir(fw.rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		if lastTime, ok := lastMod[path]; ok {
			if info.ModTime().After(lastTime) {
				lastMod[path] = info.ModTime()
				if fw.changeFn != nil {
					fw.changeFn(path)
				}
			}
		} else {
			lastMod[path] = info.ModTime()
		}
		return nil
	})
}

// Stop stops the file watcher.
func (fw *FileWatcher) Stop() {
	close(fw.stopCh)
}

// ─── Global Workspace Functions ───────────────────────────────────────────────

var (
	globalWorkspace *WorkspaceDiagnostics
	workspaceOnce   sync.Once
)

// InitGlobalWorkspace initializes the global workspace diagnostics aggregator.
func InitGlobalWorkspace(rootDir string, options ...WorkspaceOption) error {
	var err error
	workspaceOnce.Do(func() {
		globalWorkspace = NewWorkspaceDiagnostics(rootDir, options...)
		err = globalWorkspace.Start(context.Background())
	})
	return err
}

// GetGlobalWorkspace returns the global workspace diagnostics instance.
func GetGlobalWorkspace() *WorkspaceDiagnostics {
	return globalWorkspace
}

// ScanWorkspace triggers a full workspace scan.
func ScanWorkspace(ctx context.Context) error {
	if globalWorkspace == nil {
		return fmt.Errorf("workspace not initialized, call InitGlobalWorkspace first")
	}
	return globalWorkspace.ReScan(ctx)
}

// GetWorkspaceDiagnostics returns all diagnostics in the workspace.
func GetWorkspaceDiagnostics() map[string][]types.Diagnostic {
	if globalWorkspace == nil {
		return nil
	}
	return globalWorkspace.GetAllDiagnostics()
}

// GetWorkspaceStats returns workspace statistics.
func GetWorkspaceStats() WorkspaceStats {
	if globalWorkspace == nil {
		return WorkspaceStats{}
	}
	return globalWorkspace.GetStats()
}

// ExportWorkspaceSARIF exports workspace diagnostics to SARIF format.
func ExportWorkspaceSARIF(outputPath, toolName, toolVersion string) error {
	if globalWorkspace == nil {
		return fmt.Errorf("workspace not initialized")
	}
	diags := globalWorkspace.GetAllDiagnostics()
	return ExportSARIFToFile(globalWorkspace.RootDir(), diags, outputPath, toolName, toolVersion)
}

// ExportWorkspaceGitHubAnnotations exports workspace diagnostics as GitHub Actions annotations.
func ExportWorkspaceGitHubAnnotations(outputPath string) error {
	if globalWorkspace == nil {
		return fmt.Errorf("workspace not initialized")
	}
	diags := globalWorkspace.GetAllDiagnostics()
	return WriteGitHubAnnotations(globalWorkspace.RootDir(), diags, outputPath)
}