package session

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Preloader intelligently preloads context based on patterns and history.
type Preloader struct {
	memoryStore   *MemoryStore
	searchIndex   *SearchIndex
	mu            sync.RWMutex
	cache         map[string]*PreloadedContext
	cacheExpiry   time.Duration
	maxCacheItems int
}

// PreloadedContext contains preloaded context for a project.
type PreloadedContext struct {
	ProjectPath    string
	RecentFiles    []string
	PredictedFiles []string
	Conventions    map[string]Convention
	Patterns       []CodePattern
	Architecture   *ArchitectureDecisions
	LoadedAt       time.Time
	Metadata       map[string]any
}

// PreloadConfig configures preloading behavior.
type PreloadConfig struct {
	MaxRecentFiles    int
	MaxPredictedFiles int
	CacheExpiry       time.Duration
	MaxCacheItems     int
	DepthLimit        int
}

// DefaultPreloadConfig returns sensible defaults.
func DefaultPreloadConfig() PreloadConfig {
	return PreloadConfig{
		MaxRecentFiles:    10,
		MaxPredictedFiles: 5,
		CacheExpiry:       30 * time.Minute,
		MaxCacheItems:     10,
		DepthLimit:        10,
	}
}

// NewPreloader creates a new preloader.
func NewPreloader(memoryStore *MemoryStore, searchIndex *SearchIndex, config PreloadConfig) *Preloader {
	if config.CacheExpiry == 0 {
		config.CacheExpiry = 30 * time.Minute
	}
	if config.MaxCacheItems == 0 {
		config.MaxCacheItems = 10
	}
	return &Preloader{
		memoryStore:   memoryStore,
		searchIndex:   searchIndex,
		cache:         make(map[string]*PreloadedContext),
		cacheExpiry:   config.CacheExpiry,
		maxCacheItems: config.MaxCacheItems,
	}
}

// PreloadProject preloads context for a project.
func (p *Preloader) PreloadProject(projectPath string, config PreloadConfig) (*PreloadedContext, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check cache
	if cached, ok := p.cache[projectPath]; ok {
		if time.Since(cached.LoadedAt) < p.cacheExpiry {
			return cached, nil
		}
	}

	ctx := &PreloadedContext{
		ProjectPath: projectPath,
		LoadedAt:    time.Now(),
		Metadata:    make(map[string]any),
	}

	// Load from memory store
	if p.memoryStore != nil {
		pm, err := p.memoryStore.GetProject(projectPath)
		if err == nil && pm != nil {
			// Recent files
			maxRecent := config.MaxRecentFiles
			if maxRecent <= 0 {
				maxRecent = 10
			}
			for i, rf := range pm.RecentFiles {
				if i >= maxRecent {
					break
				}
				ctx.RecentFiles = append(ctx.RecentFiles, rf.Path)
			}

			// Conventions
			ctx.Conventions = pm.Conventions

			// Get most used patterns
			ctx.Patterns = p.memoryStore.GetMostUsedPatterns(projectPath, 10)

			// Architecture
			ctx.Architecture = pm.Architecture
		}
	}

	// Predict files based on current directory structure
	ctx.PredictedFiles = p.predictRelevantFiles(projectPath, config.MaxPredictedFiles, config.DepthLimit)

	// Cache the result
	p.cache[projectPath] = ctx
	p.evictOldCache()

	return ctx, nil
}

// walkWithDepth walks a directory tree with a depth limit.
func walkWithDepth(root string, maxDepth int, fn filepath.WalkFunc) error {
	if maxDepth < 0 {
		return nil
	}

	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		// Calculate depth relative to root
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}

		depth := len(strings.Split(strings.TrimPrefix(rel, "."), string(filepath.Separator)))
		if depth > maxDepth && info.IsDir() {
			return filepath.SkipDir
		}

		return fn(path, info, nil)
	})
}

// predictRelevantFiles predicts files that might be needed.
func (p *Preloader) predictRelevantFiles(projectPath string, maxFiles int, depth int) []string {
	if maxFiles <= 0 {
		maxFiles = 5
	}
	if depth <= 0 {
		depth = 10
	}

	var candidates []fileCandidate

	// Look for common entry points and config files
	importantFiles := []string{
		"README.md", "README", "readme.md",
		"Makefile", "makefile",
		"package.json", "go.mod", "Cargo.toml", "pom.xml", "build.gradle",
		"requirements.txt", "setup.py", "pyproject.toml",
		"tsconfig.json", "webpack.config.js", "vite.config.js",
		".automergent.md", "AUTOMERGENT.md",
		"main.go", "main.py", "index.js", "index.ts", "app.py", "app.js",
		"src/main.go", "src/index.ts", "src/index.js", "src/main.py",
		"cmd/main.go", "cmd/server/main.go",
	}

	for _, file := range importantFiles {
		fullPath := filepath.Join(projectPath, file)
		if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
			candidates = append(candidates, fileCandidate{
				path:     fullPath,
				priority: fileTypePriority(file),
			})
		}
	}

	// Also check for recently modified files
	recentlyModified := p.findRecentlyModified(projectPath, 20, depth)
	for _, path := range recentlyModified {
		candidates = append(candidates, fileCandidate{
			path:     path,
			priority: 5 + fileTypePriority(filepath.Base(path)),
		})
	}

	// Sort by priority and dedupe
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].priority > candidates[j].priority
	})

	seen := make(map[string]bool)
	var result []string
	for _, c := range candidates {
		if !seen[c.path] {
			seen[c.path] = true
			result = append(result, c.path)
			if len(result) >= maxFiles {
				break
			}
		}
	}

	return result
}

type fileCandidate struct {
	path     string
	priority int
}

// fileTypePriority returns a priority score based on file importance.
func fileTypePriority(filename string) int {
	lowerName := strings.ToLower(filename)

	// Config and build files (highest priority)
	if strings.Contains(lowerName, "config") ||
		lowerName == "package.json" ||
		lowerName == "go.mod" ||
		lowerName == "makefile" ||
		lowerName == "cargo.toml" {
		return 20
	}

	// Documentation
	if strings.HasPrefix(lowerName, "readme") || strings.Contains(lowerName, "automergent") {
		return 18
	}

	// Entry points
	if lowerName == "main.go" || lowerName == "main.py" ||
		lowerName == "index.js" || lowerName == "index.ts" ||
		lowerName == "app.py" || lowerName == "app.js" {
		return 15
	}

	// Source files
	ext := filepath.Ext(lowerName)
	switch ext {
	case ".go", ".rs", ".py", ".ts", ".tsx", ".js", ".jsx":
		return 10
	case ".java", ".kt", ".cpp", ".c", ".h":
		return 9
	case ".md", ".txt":
		return 5
	default:
		return 1
	}
}

// findRecentlyModified finds recently modified source files.
func (p *Preloader) findRecentlyModified(projectPath string, maxFiles int, depth int) []string {
	if depth <= 0 {
		depth = 10
	}

	type fileInfo struct {
		path    string
		modTime time.Time
	}

	var files []fileInfo
	cutoff := time.Now().Add(-7 * 24 * time.Hour) // Last week

	_ = walkWithDepth(projectPath, depth, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		// Skip hidden directories
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") || info.Name() == "node_modules" || info.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip if too old
		if info.ModTime().Before(cutoff) {
			return nil
		}

		// Only include source files
		ext := filepath.Ext(path)
		sourceExts := map[string]bool{
			".go": true, ".py": true, ".js": true, ".ts": true, ".tsx": true,
			".jsx": true, ".rs": true, ".java": true, ".rb": true, ".cpp": true,
			".c": true, ".h": true, ".cs": true, ".kt": true, ".swift": true,
		}
		if !sourceExts[ext] {
			return nil
		}

		files = append(files, fileInfo{path: path, modTime: info.ModTime()})
		return nil
	})

	// Sort by modification time (newest first)
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})

	var result []string
	for i, f := range files {
		if i >= maxFiles {
			break
		}
		result = append(result, f.path)
	}

	return result
}

// evictOldCache removes oldest cache entries if over limit.
func (p *Preloader) evictOldCache() {
	if len(p.cache) <= p.maxCacheItems {
		return
	}

	type cacheEntry struct {
		key      string
		loadedAt time.Time
	}

	var entries []cacheEntry
	for k, v := range p.cache {
		entries = append(entries, cacheEntry{k, v.LoadedAt})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].loadedAt.Before(entries[j].loadedAt)
	})

	// Remove oldest entries
	toRemove := len(entries) - p.maxCacheItems
	for i := 0; i < toRemove; i++ {
		delete(p.cache, entries[i].key)
	}
}

// GetCachedContext returns cached context if available and fresh.
func (p *Preloader) GetCachedContext(projectPath string) (*PreloadedContext, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	cached, ok := p.cache[projectPath]
	if !ok {
		return nil, false
	}

	if time.Since(cached.LoadedAt) >= p.cacheExpiry {
		return nil, false
	}

	return cached, true
}

// InvalidateCache removes a project from the cache.
func (p *Preloader) InvalidateCache(projectPath string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.cache, projectPath)
}

// ClearCache clears all cached contexts.
func (p *Preloader) ClearCache() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cache = make(map[string]*PreloadedContext)
}

// WarmCache preloads context for recently used projects.
func (p *Preloader) WarmCache(maxProjects int) {
	if p.memoryStore == nil {
		return
	}

	recentProjects := p.memoryStore.GetRecentProjects(maxProjects)
	config := DefaultPreloadConfig()

	for _, rp := range recentProjects {
		// Skip if directory doesn't exist
		if _, err := os.Stat(rp.Path); os.IsNotExist(err) {
			continue
		}
		_, _ = p.PreloadProject(rp.Path, config)
	}
}

// SmartDefaults generates smart defaults based on project analysis.
func (p *Preloader) SmartDefaults(projectPath string) map[string]string {
	defaults := make(map[string]string)

	// Detect project type
	defaults["project_type"] = detectProjectType(projectPath)

	// Detect build system
	defaults["build_system"] = detectBuildSystem(projectPath)

	// Detect test framework
	defaults["test_framework"] = detectTestFramework(projectPath)

	// Detect primary language
	defaults["primary_language"] = detectPrimaryLanguage(projectPath)

	return defaults
}

// detectProjectType identifies the type of project.
func detectProjectType(projectPath string) string {
	files := map[string]string{
		"package.json":     "nodejs",
		"go.mod":           "go",
		"Cargo.toml":       "rust",
		"pom.xml":          "java",
		"build.gradle":     "java",
		"requirements.txt": "python",
		"setup.py":         "python",
		"pyproject.toml":   "python",
		"Gemfile":          "ruby",
		"mix.exs":          "elixir",
		"pubspec.yaml":     "dart",
	}

	for file, projectType := range files {
		if _, err := os.Stat(filepath.Join(projectPath, file)); err == nil {
			return projectType
		}
	}

	return "unknown"
}

// detectBuildSystem identifies the build system.
func detectBuildSystem(projectPath string) string {
	buildSystems := map[string]string{
		"Makefile":         "make",
		"makefile":         "make",
		"CMakeLists.txt":   "cmake",
		"build.gradle":     "gradle",
		"build.gradle.kts": "gradle",
		"pom.xml":          "maven",
		"package.json":     "npm",
		"yarn.lock":        "yarn",
		"pnpm-lock.yaml":   "pnpm",
		"Cargo.toml":       "cargo",
		"go.mod":           "go",
		"setup.py":         "setuptools",
		"pyproject.toml":   "poetry/pip",
		"requirements.txt": "pip",
		"mix.exs":          "mix",
		"pubspec.yaml":     "pub",
	}

	for file, system := range buildSystems {
		if _, err := os.Stat(filepath.Join(projectPath, file)); err == nil {
			return system
		}
	}

	return "unknown"
}

// detectTestFramework identifies the test framework.
func detectTestFramework(projectPath string) string {
	// Check package.json for JavaScript testing frameworks
	pkgJSON := filepath.Join(projectPath, "package.json")
	if data, err := os.ReadFile(pkgJSON); err == nil {
		content := string(data)
		if strings.Contains(content, "jest") {
			return "jest"
		}
		if strings.Contains(content, "mocha") {
			return "mocha"
		}
		if strings.Contains(content, "vitest") {
			return "vitest"
		}
	}

	// Go testing
	if _, err := os.Stat(filepath.Join(projectPath, "go.mod")); err == nil {
		return "go test"
	}

	// Python testing
	if _, err := os.Stat(filepath.Join(projectPath, "pytest.ini")); err == nil {
		return "pytest"
	}
	if _, err := os.Stat(filepath.Join(projectPath, "setup.cfg")); err == nil {
		return "pytest"
	}

	// Rust testing
	if _, err := os.Stat(filepath.Join(projectPath, "Cargo.toml")); err == nil {
		return "cargo test"
	}

	return "unknown"
}

// detectPrimaryLanguage identifies the primary programming language.
func detectPrimaryLanguage(projectPath string) string {
	languageFiles := map[string]string{
		".go":    "go",
		".py":    "python",
		".js":    "javascript",
		".ts":    "typescript",
		".tsx":   "typescript",
		".rs":    "rust",
		".java":  "java",
		".kt":    "kotlin",
		".rb":    "ruby",
		".cpp":   "c++",
		".c":     "c",
		".cs":    "csharp",
		".swift": "swift",
	}

	counts := make(map[string]int)

	_ = walkWithDepth(projectPath, 10, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}

		ext := filepath.Ext(path)
		if lang, ok := languageFiles[ext]; ok {
			counts[lang]++
		}
		return nil
	})

	// Find most common language
	maxLang := "unknown"
	maxCount := 0
	for lang, count := range counts {
		if count > maxCount {
			maxCount = count
			maxLang = lang
		}
	}

	return maxLang
}
