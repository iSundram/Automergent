package context

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// DependencyGraph represents file dependencies.
type DependencyGraph struct {
	mu           sync.RWMutex
	dependencies map[string][]string // file -> files it depends on
	dependents   map[string][]string // file -> files that depend on it
	imports      map[string][]string // file -> import paths
}

// NewDependencyGraph creates a new dependency graph.
func NewDependencyGraph() *DependencyGraph {
	return &DependencyGraph{
		dependencies: make(map[string][]string),
		dependents:   make(map[string][]string),
		imports:      make(map[string][]string),
	}
}

// AddDependency adds a dependency relationship.
func (dg *DependencyGraph) AddDependency(from, to string) {
	dg.mu.Lock()
	defer dg.mu.Unlock()

	if !containsString(dg.dependencies[from], to) {
		dg.dependencies[from] = append(dg.dependencies[from], to)
	}
	if !containsString(dg.dependents[to], from) {
		dg.dependents[to] = append(dg.dependents[to], from)
	}
}

// SetDependencies replaces all dependencies for a file.
func (dg *DependencyGraph) SetDependencies(file string, deps []string) {
	dg.mu.Lock()
	defer dg.mu.Unlock()

	// Remove old dependents entries
	if old, exists := dg.dependencies[file]; exists {
		for _, dep := range old {
			dg.removeFromSlice(dg.dependents, dep, file)
		}
	}

	// Store a copy of deps to avoid external mutation
	dg.dependencies[file] = append([]string(nil), deps...)

	// Update dependents
	for _, dep := range deps {
		if !containsString(dg.dependents[dep], file) {
			dg.dependents[dep] = append(dg.dependents[dep], file)
		}
	}
}

// SetImports stores import paths for a file.
func (dg *DependencyGraph) SetImports(file string, imports []string) {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	dg.imports[file] = imports
}

// GetDependencies returns direct dependencies of a file.
func (dg *DependencyGraph) GetDependencies(file string) []string {
	dg.mu.RLock()
	defer dg.mu.RUnlock()
	orig := dg.dependencies[file]
	if orig == nil {
		return nil
	}
	out := make([]string, len(orig))
	copy(out, orig)
	return out
}

// GetDependents returns files that depend on this file.
func (dg *DependencyGraph) GetDependents(file string) []string {
	dg.mu.RLock()
	defer dg.mu.RUnlock()
	orig := dg.dependents[file]
	if orig == nil {
		return nil
	}
	out := make([]string, len(orig))
	copy(out, orig)
	return out
}

// GetImports returns import paths for a file.
func (dg *DependencyGraph) GetImports(file string) []string {
	dg.mu.RLock()
	defer dg.mu.RUnlock()
	orig := dg.imports[file]
	if orig == nil {
		return nil
	}
	out := make([]string, len(orig))
	copy(out, orig)
	return out
}

// TransitiveClosure returns all files reachable from the given files.
func (dg *DependencyGraph) TransitiveClosure(files []string, direction Direction, maxDepth int) []string {
	dg.mu.RLock()
	defer dg.mu.RUnlock()

	visited := make(map[string]bool)
	result := make([]string, 0)

	var visit func(file string, depth int)
	visit = func(file string, depth int) {
		if visited[file] || (maxDepth > 0 && depth > maxDepth) {
			return
		}
		visited[file] = true
		result = append(result, file)

		var next []string
		switch direction {
		case DirectionDependencies:
			next = dg.dependencies[file]
		case DirectionDependents:
			next = dg.dependents[file]
		case DirectionBoth:
			next = append(dg.dependencies[file], dg.dependents[file]...)
		}

		for _, f := range next {
			visit(f, depth+1)
		}
	}

	for _, f := range files {
		visit(f, 0)
	}

	return result
}

// Direction specifies which direction to traverse.
type Direction int

const (
	DirectionDependencies Direction = iota
	DirectionDependents
	DirectionBoth
)

// MinimalSufficientSet computes the smallest set of files needed.
func (dg *DependencyGraph) MinimalSufficientSet(targets []string, availableBudget int, prioritizer func([]string) []string) []string {
	// Get all required files
	required := dg.TransitiveClosure(targets, DirectionDependencies, 0)

	// Add any files that these depend on for compilation/understanding
	allDeps := make(map[string]bool)
	for _, f := range required {
		allDeps[f] = true
		for _, dep := range dg.dependencies[f] {
			allDeps[dep] = true
		}
	}

	result := make([]string, 0, len(allDeps))
	for f := range allDeps {
		result = append(result, f)
	}

	// Prioritize if function provided
	if prioritizer != nil {
		result = prioritizer(result)
	}

	return result
}

// removeFromSlice removes a value from a slice in a map.
func (dg *DependencyGraph) removeFromSlice(m map[string][]string, key, val string) {
	slice := m[key]
	for i, v := range slice {
		if v == val {
			m[key] = append(slice[:i], slice[i+1:]...)
			break
		}
	}
}

// Remove removes a file from the graph.
func (dg *DependencyGraph) Remove(file string) {
	dg.mu.Lock()
	defer dg.mu.Unlock()

	// Remove from dependents of its dependencies
	if deps, exists := dg.dependencies[file]; exists {
		for _, dep := range deps {
			dg.removeFromSlice(dg.dependents, dep, file)
		}
	}

	// Remove from dependencies of its dependents
	if deps, exists := dg.dependents[file]; exists {
		for _, dep := range deps {
			dg.removeFromSlice(dg.dependencies, dep, file)
		}
	}

	delete(dg.dependencies, file)
	delete(dg.dependents, file)
	delete(dg.imports, file)
}

// DependencyAnalyzer analyzes file dependencies.
type DependencyAnalyzer struct {
	graph   *DependencyGraph
	rootDir string
	parsers map[string]ImportParser
}

// ImportParser parses imports from a file.
type ImportParser interface {
	ParseImports(content string) ([]string, error)
	Extensions() []string
}

// NewDependencyAnalyzer creates a new analyzer.
func NewDependencyAnalyzer(rootDir string) *DependencyAnalyzer {
	return &DependencyAnalyzer{
		graph:   NewDependencyGraph(),
		rootDir: rootDir,
		parsers: make(map[string]ImportParser),
	}
}

// RegisterParser registers an import parser for file extensions.
func (da *DependencyAnalyzer) RegisterParser(parser ImportParser) {
	for _, ext := range parser.Extensions() {
		da.parsers[ext] = parser
	}
}

// Graph returns the dependency graph.
func (da *DependencyAnalyzer) Graph() *DependencyGraph {
	return da.graph
}

// AnalyzeFile analyzes dependencies for a single file.
func (da *DependencyAnalyzer) AnalyzeFile(ctx context.Context, path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	ext := filepath.Ext(path)
	parser, exists := da.parsers[ext]
	if !exists {
		return nil // No parser for this extension
	}

	imports, err := parser.ParseImports(string(content))
	if err != nil {
		return err
	}

	// Resolve imports to file paths
	resolved := da.resolveImports(path, imports)

	da.graph.SetDependencies(path, resolved)
	da.graph.SetImports(path, imports)

	return nil
}

// AnalyzeDirectory analyzes all files in a directory.
func (da *DependencyAnalyzer) AnalyzeDirectory(ctx context.Context, dir string) error {
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err // Propagate errors so callers can report them
		}

		if d.IsDir() {
			// Skip common non-source directories
			name := d.Name()
			if name == "node_modules" || name == ".git" || name == "vendor" ||
				name == "__pycache__" || name == ".cache" {
				return filepath.SkipDir
			}
			return nil
		}

		ext := filepath.Ext(path)
		if _, exists := da.parsers[ext]; exists {
			return da.AnalyzeFile(ctx, path)
		}

		return nil
	})
}

// resolveImports converts import paths to file paths.
func (da *DependencyAnalyzer) resolveImports(fromFile string, imports []string) []string {
	var resolved []string
	dir := filepath.Dir(fromFile)
	rootClean := filepath.Clean(da.rootDir)

	for _, imp := range imports {
		// Handle relative imports
		if strings.HasPrefix(imp, ".") {
			path := filepath.Join(dir, imp)
			for _, f := range da.findFile(path) {
				if isWithinRoot(rootClean, f) {
					resolved = append(resolved, f)
				}
			}
		} else {
			// Handle absolute/module imports
			path := filepath.Join(da.rootDir, imp)
			for _, f := range da.findFile(path) {
				if isWithinRoot(rootClean, f) {
					resolved = append(resolved, f)
				}
			}
		}
	}

	return resolved
}

// isWithinRoot checks that a candidate path is inside the workspace root.
func isWithinRoot(root, candidate string) bool {
	if root == "" {
		return true
	}
	root = filepath.Clean(root)
	cand := filepath.Clean(candidate)
	if cand == root {
		return true
	}
	// Ensure trailing separator on root for prefix checking
	rootPrefix := root + string(filepath.Separator)
	return strings.HasPrefix(cand, rootPrefix)
}

// findFile finds actual file(s) from an import path.
func (da *DependencyAnalyzer) findFile(basePath string) []string {
	var found []string
	rootClean := filepath.Clean(da.rootDir)

	// Helper to validate and add candidate
	addIfExists := func(path string) {
		cand := filepath.Clean(path)
		if !isWithinRoot(rootClean, cand) {
			return
		}
		if _, err := os.Stat(cand); err == nil {
			found = append(found, cand)
		}
	}

	// Try exact match
	addIfExists(basePath)
	if len(found) > 0 {
		return found
	}

	// Try common extensions
	extensions := []string{".go", ".ts", ".tsx", ".js", ".jsx", ".py", ".rs"}
	for _, ext := range extensions {
		addIfExists(basePath + ext)
	}

	// Try index files
	indexFiles := []string{"index.ts", "index.tsx", "index.js", "mod.rs", "__init__.py"}
	for _, idx := range indexFiles {
		addIfExists(filepath.Join(basePath, idx))
	}

	return found
}

// GoImportParser parses Go import statements.
type GoImportParser struct{}

func (p *GoImportParser) Extensions() []string {
	return []string{".go"}
}

func (p *GoImportParser) ParseImports(content string) ([]string, error) {
	var imports []string
	lines := strings.Split(content, "\n")
	inImportBlock := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "import (") {
			inImportBlock = true
			continue
		}
		if inImportBlock && line == ")" {
			inImportBlock = false
			continue
		}
		if strings.HasPrefix(line, "import ") {
			// Single import
			imp := extractQuotedString(line)
			if imp != "" {
				imports = append(imports, imp)
			}
			continue
		}
		if inImportBlock {
			imp := extractQuotedString(line)
			if imp != "" {
				imports = append(imports, imp)
			}
		}
	}

	return imports, nil
}

// TypeScriptImportParser parses TypeScript/JavaScript imports.
type TypeScriptImportParser struct{}

func (p *TypeScriptImportParser) Extensions() []string {
	return []string{".ts", ".tsx", ".js", ".jsx"}
}

func (p *TypeScriptImportParser) ParseImports(content string) ([]string, error) {
	var imports []string
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// import ... from '...'
		if strings.HasPrefix(line, "import ") {
			if idx := strings.Index(line, " from "); idx != -1 {
				imp := extractQuotedString(line[idx+6:])
				if imp != "" {
					imports = append(imports, imp)
				}
			} else {
				// import '...' (side effect import)
				imp := extractQuotedString(line[7:])
				if imp != "" {
					imports = append(imports, imp)
				}
			}
		}
		// require('...')
		if strings.Contains(line, "require(") {
			start := strings.Index(line, "require(")
			imp := extractQuotedString(line[start+8:])
			if imp != "" {
				imports = append(imports, imp)
			}
		}
	}

	return imports, nil
}

// PythonImportParser parses Python imports.
type PythonImportParser struct{}

func (p *PythonImportParser) Extensions() []string {
	return []string{".py"}
}

func (p *PythonImportParser) ParseImports(content string) ([]string, error) {
	var imports []string
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// from x import y
		if strings.HasPrefix(line, "from ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				imports = append(imports, parts[1])
			}
		}
		// import x
		if strings.HasPrefix(line, "import ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				// Handle multiple imports: import a, b, c
				for _, part := range parts[1:] {
					part = strings.Trim(part, ",")
					if part != "" {
						imports = append(imports, part)
					}
				}
			}
		}
	}

	return imports, nil
}

func extractQuotedString(s string) string {
	// Find string in single or double quotes
	for _, q := range []byte{'"', '\'', '`'} {
		start := strings.IndexByte(s, q)
		if start == -1 {
			continue
		}
		end := strings.IndexByte(s[start+1:], q)
		if end != -1 {
			return s[start+1 : start+1+end]
		}
	}
	return ""
}

// ContextSelector selects relevant context files.
type ContextSelector struct {
	graph        *DependencyGraph
	ranker       *Ranker
	budget       *TokenBudget
	detector     *StalenessDetector
	fileProvider func(context.Context, string) (string, bool)
}

// ContextSignal captures the signals used to select and rank a file.
type ContextSignal struct {
	Path            string  `json:"path"`
	Score           float64 `json:"score"`
	Required        bool    `json:"required"`
	DependencyDepth int     `json:"dependency_depth"`
	Freshness       float64 `json:"freshness"`
	Staleness       string  `json:"staleness"`
	Modified        bool    `json:"modified"`
}

// NewContextSelector creates a new context selector.
func NewContextSelector(graph *DependencyGraph, ranker *Ranker, budget *TokenBudget, detector *StalenessDetector) *ContextSelector {
	return &ContextSelector{
		graph:    graph,
		ranker:   ranker,
		budget:   budget,
		detector: detector,
	}
}

// SetFileProvider sets a function to fetch file content (e.g., from Manager cache).
func (cs *ContextSelector) SetFileProvider(fn func(context.Context, string) (string, bool)) {
	cs.fileProvider = fn
}

// SelectContext selects the most relevant context files.
func (cs *ContextSelector) SelectContext(ctx context.Context, intent string, activeFiles []string, tokenBudget int, includeDeps bool) ([]ContextItem, []ContextSignal, error) {
	// 1. Start with active files and optionally traverse dependencies.
	direction := DirectionDependencies
	if includeDeps {
		direction = DirectionBoth
	}
	targetFiles := cs.graph.TransitiveClosure(activeFiles, direction, 3)

	// 2. Get file contexts
	var fileContexts []FileContext
	signals := make(map[string]ContextSignal, len(targetFiles))
	for _, path := range targetFiles {
		var content string
		if cs.fileProvider != nil {
			var ok bool
			content, ok = cs.fileProvider(ctx, path)
			if !ok {
				continue
			}
		} else {
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			content = string(data)
		}

		fc := FileContext{
			Path:         path,
			Content:      content,
			Dependencies: cs.graph.GetDependencies(path),
			Dependents:   cs.graph.GetDependents(path),
			Symbols:      extractSymbols(content, filepath.Ext(path)),
		}
		required := containsString(activeFiles, path)
		fc.FreshnessState = "fresh"

		// Check staleness
		if cs.detector != nil {
			status, err := cs.detector.Check(ctx, path)
			if err == nil && status.State == StateModified {
				fc.IsModified = true
			}
			if err == nil {
				fc.ModTime = status.ModTime
				switch status.State {
				case StateFresh:
					fc.Freshness = 1
					fc.FreshnessState = status.State.String()
				case StateModified:
					fc.Freshness = 0.6
					fc.Staleness = 0.35
					fc.FreshnessState = status.State.String()
				case StateStale:
					fc.Freshness = 0.25
					fc.Staleness = 0.7
					fc.FreshnessState = status.State.String()
				case StateInvalid:
					fc.Freshness = 0
					fc.Staleness = 1
					fc.FreshnessState = status.State.String()
				}
			}
		}

		fileContexts = append(fileContexts, fc)
		depDepth := 0
		if len(fc.Dependencies) > 0 {
			depDepth = 1
		}
		signals[path] = ContextSignal{
			Path:            path,
			Required:        required,
			DependencyDepth: depDepth,
			Freshness:       fc.Freshness,
			Staleness:       fc.FreshnessState,
			Modified:        fc.IsModified,
		}
	}

	// 3. Rank files by relevance
	scores := cs.ranker.RankFiles(fileContexts, intent, 0)
	scoreByPath := make(map[string]RelevanceScore, len(scores))
	for _, score := range scores {
		scoreByPath[score.Path] = score
	}

	// 4. Build context items with token counts
	var items []ContextItem
	for _, fc := range fileContexts {
		score, ok := scoreByPath[fc.Path]
		if !ok {
			continue
		}
		sig := signals[fc.Path]
		sig.Score = score.Score
		signals[fc.Path] = sig
		tokens := EstimateTokens(fc.Content)

		items = append(items, ContextItem{
			Path:       score.Path,
			Content:    fc.Content,
			Tokens:     tokens,
			Priority:   score.Score,
			Required:   signals[fc.Path].Required,
			Freshness:  fc.Freshness,
			Dependency: float64(len(fc.Dependencies)),
			Staleness:  fc.FreshnessState,
		})
	}

	// 5. Fit to budget
	truncator := NewTruncator(TruncateSummarize)
	items, err := truncator.FitToBudget(items, tokenBudget)
	if err != nil {
		return nil, nil, err
	}
	return items, valuesSignals(signals), nil
}

// SelectContextWithPriority selects context files with lazy-loading support.
func (cs *ContextSelector) SelectContextWithPriority(
	ctx context.Context,
	intent string,
	activeFiles []string,
	tokenBudget int,
	includeDeps bool,
	maxLazyLevel ContextPriority,
) ([]ContextItem, []ContextSignal, error) {
	items, signals, err := cs.SelectContext(ctx, intent, activeFiles, tokenBudget, includeDeps)
	if err != nil {
		return nil, nil, err
	}

	for i := range items {
		item := &items[i]
		if item.Required {
			item.PriorityLevel = PriorityCritical
		} else if item.Priority > 0.7 {
			item.PriorityLevel = PriorityHigh
		} else if item.Priority > 0.4 {
			item.PriorityLevel = PriorityMedium
		} else if item.Priority > 0.2 {
			item.PriorityLevel = PriorityLow
		} else {
			item.PriorityLevel = PriorityLazy
		}
	}

	var filtered []ContextItem
	for _, item := range items {
		if item.PriorityLevel <= maxLazyLevel {
			filtered = append(filtered, item)
		}
	}

	return filtered, signals, nil
}

// SelectMinimalContext selects the minimal sufficient context.
func (cs *ContextSelector) SelectMinimalContext(targets []string, tokenBudget int) ([]ContextItem, error) {
	// Get minimal set of required files
	minimal := cs.graph.MinimalSufficientSet(targets, tokenBudget, nil)

	// Sort by importance (dependents count)
	sort.Slice(minimal, func(i, j int) bool {
		depsI := len(cs.graph.GetDependents(minimal[i]))
		depsJ := len(cs.graph.GetDependents(minimal[j]))
		return depsI > depsJ
	})

	// Build items
	var items []ContextItem
	for _, path := range minimal {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		tokens := EstimateTokens(string(content))
		items = append(items, ContextItem{
			Path:       path,
			Content:    string(content),
			Tokens:     tokens,
			Priority:   float64(len(cs.graph.GetDependents(path))) / 10.0,
			Required:   true,
			Dependency: float64(len(cs.graph.GetDependencies(path))),
		})
	}

	// Fit to budget
	truncator := NewTruncator(TruncateLowestPriority)
	return truncator.FitToBudget(items, tokenBudget)
}

func valuesSignals(m map[string]ContextSignal) []ContextSignal {
	out := make([]ContextSignal, 0, len(m))
	for _, sig := range m {
		out = append(out, sig)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Required != out[j].Required {
			return out[i].Required
		}
		if out[i].Freshness != out[j].Freshness {
			return out[i].Freshness > out[j].Freshness
		}
		return out[i].Path < out[j].Path
	})
	return out
}
