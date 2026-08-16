package context

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/iSundram/Automergent/internal/ai"
)

// MemoryTier represents a tier in the JIT memory hierarchy.
type MemoryTier int

const (
	TierGlobal MemoryTier = iota // ~/.automergent/*.md
	TierProject                  // AUTOMERGENT.md in project root
	TierSubdir                   // .automergent.md in subdirectories (JIT)
)

// JITMemoryLoader handles tiered, just-in-time loading of memory files.
type JITMemoryLoader struct {
	mu           sync.RWMutex
	rootDir      string
	globalPaths  []string
	projectPaths map[string][]string // projectDir -> paths
	subdirCache  map[string]*SubdirMemory
	loaded       map[string]bool     // path -> loaded
	accessLog    []MemoryAccess
}

// SubdirMemory holds memory for a subdirectory.
type SubdirMemory struct {
	Path      string
	Content   string
	LoadedAt  time.Time
	AccessCount int
}

// MemoryAccess tracks access for LRU.
type MemoryAccess struct {
	Path      string
	Tier      MemoryTier
	Timestamp time.Time
}

// NewJITMemoryLoader creates a JIT memory loader.
func NewJITMemoryLoader(rootDir string) *JITMemoryLoader {
	return &JITMemoryLoader{
		rootDir:      rootDir,
		projectPaths: make(map[string][]string),
		subdirCache:  make(map[string]*SubdirMemory),
		loaded:       make(map[string]bool),
		accessLog:    make([]MemoryAccess, 0, 1000),
	}
}

// LoadGlobal loads global memory files (~/.automergent/*.md).
func (j *JITMemoryLoader) LoadGlobal() ([]string, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	if len(j.globalPaths) > 0 {
		return j.globalPaths, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	globalDir := filepath.Join(home, ".automergent")
	entries, err := os.ReadDir(globalDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(globalDir, e.Name())
		j.globalPaths = append(j.globalPaths, path)
		j.loaded[path] = true
	}
	return j.globalPaths, nil
}

// LoadProject loads project-level memory (AUTOMERGENT.md, .claude/CLAUDE.md, etc.).
func (j *JITMemoryLoader) LoadProject(projectDir string) ([]string, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	if paths, ok := j.projectPaths[projectDir]; ok {
		return paths, nil
	}

	var paths []string
	// Standard memory files
	candidates := []string{
		"AUTOMERGENT.md",
		".automergent.md",
		"CLAUDE.md",
		".claude/CLAUDE.md",
		".claude/rules/*.md",
		"AGENTS.md",
		"AGENTS.local.md",
	}

	for _, cand := range candidates {
		if strings.Contains(cand, "*") {
			// Glob pattern
			matches, _ := filepath.Glob(filepath.Join(projectDir, cand))
			paths = append(paths, matches...)
		} else {
			path := filepath.Join(projectDir, cand)
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				paths = append(paths, path)
			}
		}
	}

	j.projectPaths[projectDir] = paths
	for _, p := range paths {
		j.loaded[p] = true
	}
	return paths, nil
}

// LoadSubdir loads subdirectory memory on demand (JIT).
func (j *JITMemoryLoader) LoadSubdir(projectDir, subdir string) (*SubdirMemory, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	key := filepath.Join(projectDir, subdir)
	if cached, ok := j.subdirCache[key]; ok {
		cached.AccessCount++
		j.recordAccess(key, TierSubdir)
		return cached, nil
	}

	// Look for .automergent.md or similar in subdir
	candidates := []string{
		".automergent.md",
		"automergent.md",
		"README.md",
		".claude/instructions.md",
	}

	for _, cand := range candidates {
		path := filepath.Join(projectDir, subdir, cand)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			content, _ := os.ReadFile(path)
			mem := &SubdirMemory{
				Path:        path,
				Content:     string(content),
				LoadedAt:    time.Now(),
				AccessCount: 1,
			}
			j.subdirCache[key] = mem
			j.loaded[path] = true
			j.recordAccess(key, TierSubdir)
			return mem, nil
		}
	}

	return nil, nil
}

// GetAllLoaded returns all loaded memory content concatenated.
func (j *JITMemoryLoader) GetAllLoaded(projectDir string) (string, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()

	var parts []string

	// Global
	for _, p := range j.globalPaths {
		if content, err := os.ReadFile(p); err == nil {
			parts = append(parts, fmt.Sprintf("# Global: %s\n%s", filepath.Base(p), string(content)))
		}
	}

	// Project
	for _, p := range j.projectPaths[projectDir] {
		if content, err := os.ReadFile(p); err == nil {
			parts = append(parts, fmt.Sprintf("# Project: %s\n%s", filepath.Base(p), string(content)))
		}
	}

	// Subdir (only if accessed recently)
	cutoff := time.Now().Add(-30 * time.Minute)
	for _, mem := range j.subdirCache {
		if mem.LoadedAt.After(cutoff) || mem.AccessCount > 2 {
			parts = append(parts, fmt.Sprintf("# Subdir: %s\n%s", mem.Path, mem.Content))
		}
	}

	return strings.Join(parts, "\n\n---\n\n"), nil
}

func (j *JITMemoryLoader) recordAccess(path string, tier MemoryTier) {
	j.accessLog = append(j.accessLog, MemoryAccess{
		Path:      path,
		Tier:      tier,
		Timestamp: time.Now(),
	})
	if len(j.accessLog) > 1000 {
		j.accessLog = j.accessLog[len(j.accessLog)-1000:]
	}
}

// EvictOld removes old subdir cache entries.
func (j *JITMemoryLoader) EvictOld() {
	j.mu.Lock()
	defer j.mu.Unlock()

	cutoff := time.Now().Add(-1 * time.Hour)
	for k, mem := range j.subdirCache {
		if mem.LoadedAt.Before(cutoff) && mem.AccessCount < 2 {
			delete(j.subdirCache, k)
		}
	}
}

// --- Diff World State ---

// WorldState tracks project context state for diff-based rendering.
type WorldState struct {
	mu           sync.RWMutex
	baseline     *WorldStateSnapshot // last emitted snapshot
	current      *WorldStateSnapshot
	sections     map[string]string   // section name -> content
	projectDir   string
}

// WorldStateSnapshot is a point-in-time capture of world state.
type WorldStateSnapshot struct {
	Sections    map[string]string `json:"sections"`
	ProjectDir  string            `json:"project_dir"`
	GitCommit   string            `json:"git_commit,omitempty"`
	Timestamp   time.Time         `json:"timestamp"`
	TokenCount  int               `json:"token_count"`
}

// NewWorldState creates a world state tracker.
func NewWorldState(projectDir string) *WorldState {
	return &WorldState{
		sections:   make(map[string]string),
		projectDir: projectDir,
		current: &WorldStateSnapshot{
			Sections:   make(map[string]string),
			ProjectDir: projectDir,
			Timestamp:  time.Now(),
		},
	}
}

// UpdateSection updates a world state section.
func (ws *WorldState) UpdateSection(name, content string) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.current.Sections[name] = content
	ws.current.Timestamp = time.Now()
}

// GetDiff returns only the sections that changed since baseline.
func (ws *WorldState) GetDiff() map[string]string {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	if ws.baseline == nil {
		// First time: emit everything
		diff := make(map[string]string)
		for k, v := range ws.current.Sections {
			diff[k] = v
		}
		return diff
	}

	diff := make(map[string]string)
	for k, v := range ws.current.Sections {
		if ws.baseline.Sections[k] != v {
			diff[k] = v
		}
	}
	return diff
}

// CommitDiff marks the current state as the new baseline.
func (ws *WorldState) CommitDiff() {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	// Deep copy
	sections := make(map[string]string, len(ws.current.Sections))
	for k, v := range ws.current.Sections {
		sections[k] = v
	}
	ws.baseline = &WorldStateSnapshot{
		Sections:   sections,
		ProjectDir: ws.current.ProjectDir,
		Timestamp:  ws.current.Timestamp,
		TokenCount: ws.current.TokenCount,
	}
}

// CurrentSnapshot returns a copy of the current snapshot.
func (ws *WorldState) CurrentSnapshot() *WorldStateSnapshot {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	sections := make(map[string]string, len(ws.current.Sections))
	for k, v := range ws.current.Sections {
		sections[k] = v
	}
	return &WorldStateSnapshot{
		Sections:   sections,
		ProjectDir: ws.current.ProjectDir,
		Timestamp:  ws.current.Timestamp,
		TokenCount: ws.current.TokenCount,
	}
}

// ToMessages renders the diff as context messages.
func (ws *WorldState) ToMessages() []ai.Message {
	diff := ws.GetDiff()
	if len(diff) == 0 {
		return nil
	}

	var parts []string
	for name, content := range diff {
		parts = append(parts, fmt.Sprintf("## %s\n%s", name, content))
	}
	content := strings.Join(parts, "\n\n---\n\n")

	return []ai.Message{
		{
			Role: ai.RoleSystem,
			Content: []ai.ContentPart{{
				Type: ai.ContentTypeText,
				Text: "# World State Update (diff)\n\n" + content,
			}},
		},
	}
}

// --- Durable Symbol/Keyword Index ---

// DurableIndex persists the ranker's symbol/keyword index to disk.
type DurableIndex struct {
	mu       sync.RWMutex
	rootDir  string
	symbols  map[string][]string // symbol -> file paths
	keywords map[string][]string // keyword -> file paths
	paths    map[string][]string // file path -> symbols
	dirty    bool
}

// NewDurableIndex creates a durable index with disk persistence.
func NewDurableIndex(rootDir string) *DurableIndex {
	dir := filepath.Join(rootDir, ".automergent", "index")
	return &DurableIndex{
		rootDir:  dir,
		symbols:  make(map[string][]string),
		keywords: make(map[string][]string),
		paths:    make(map[string][]string),
	}
}

// IndexSymbols adds symbols for a file (thread-safe).
func (di *DurableIndex) IndexSymbols(path string, symbols []string) {
	di.mu.Lock()
	defer di.mu.Unlock()

	// Remove old entries
	if old, ok := di.paths[path]; ok {
		for _, sym := range old {
			di.removeFromSlice(di.symbols, sym, path)
		}
	}

	di.paths[path] = symbols
	for _, sym := range symbols {
		if !di.contains(di.symbols[sym], path) {
			di.symbols[sym] = append(di.symbols[sym], path)
		}
	}
	di.dirty = true
}

// IndexKeywords adds keywords for a file.
func (di *DurableIndex) IndexKeywords(path string, keywords []string) {
	di.mu.Lock()
	defer di.mu.Unlock()

	for _, kw := range keywords {
		kwLower := strings.ToLower(kw)
		if !di.contains(di.keywords[kwLower], path) {
			di.keywords[kwLower] = append(di.keywords[kwLower], path)
		}
	}
	di.dirty = true
}

// FindBySymbol returns files containing a symbol.
func (di *DurableIndex) FindBySymbol(symbol string) []string {
	di.mu.RLock()
	defer di.mu.RUnlock()
	return di.symbols[symbol]
}

// FindByKeyword returns files matching a keyword.
func (di *DurableIndex) FindByKeyword(keyword string) []string {
	di.mu.RLock()
	defer di.mu.RUnlock()
	return di.keywords[strings.ToLower(keyword)]
}

// GetSymbolsForPath returns symbols for a file.
func (di *DurableIndex) GetSymbolsForPath(path string) []string {
	di.mu.RLock()
	defer di.mu.RUnlock()
	return di.paths[path]
}

// Save persists the index to disk.
func (di *DurableIndex) Save() error {
	di.mu.Lock()
	if !di.dirty {
		di.mu.Unlock()
		return nil
	}
	di.dirty = false
	di.mu.Unlock()

	data, err := json.MarshalIndent(struct {
		Symbols  map[string][]string `json:"symbols"`
		Keywords map[string][]string `json:"keywords"`
		Paths    map[string][]string `json:"paths"`
	}{
		Symbols:  di.symbols,
		Keywords: di.keywords,
		Paths:    di.paths,
	}, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(di.rootDir, 0o700); err != nil {
		return err
	}

	return atomicWriteFile(filepath.Join(di.rootDir, "durable_index.json"), data, 0o600)
}

// Load reads the index from disk.
func (di *DurableIndex) Load() error {
	di.mu.Lock()
	defer di.mu.Unlock()

	path := filepath.Join(di.rootDir, "durable_index.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var idx struct {
		Symbols  map[string][]string `json:"symbols"`
		Keywords map[string][]string `json:"keywords"`
		Paths    map[string][]string `json:"paths"`
	}
	if err := json.Unmarshal(data, &idx); err != nil {
		return err
	}

	di.symbols = idx.Symbols
	di.keywords = idx.Keywords
	di.paths = idx.Paths
	return nil
}

func (di *DurableIndex) contains(slice []string, val string) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}

func (di *DurableIndex) removeFromSlice(m map[string][]string, key, val string) {
	slice := m[key]
	for i, v := range slice {
		if v == val {
			m[key] = append(slice[:i], slice[i+1:]...)
			break
		}
	}
}