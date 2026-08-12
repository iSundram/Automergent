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

// ProjectMemory stores learned patterns and conventions for a project.
type ProjectMemory struct {
	ProjectPath   string                 `json:"project_path"`
	ProjectName   string                 `json:"project_name"`
	Conventions   map[string]Convention  `json:"conventions"`
	CodePatterns  []CodePattern          `json:"code_patterns"`
	Architecture  *ArchitectureDecisions `json:"architecture,omitempty"`
	TeamStandards map[string]string      `json:"team_standards,omitempty"`
	FilePatterns  map[string]FilePattern `json:"file_patterns,omitempty"`
	Preferences   map[string]string      `json:"preferences,omitempty"`
	RecentFiles   []RecentFile           `json:"recent_files,omitempty"`
	Tags          []string               `json:"tags,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
	AccessCount   int                    `json:"access_count"`
}

// Convention represents a learned coding convention.
type Convention struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Examples    []string  `json:"examples,omitempty"`
	Source      string    `json:"source,omitempty"` // Where it was learned from
	Confidence  float64   `json:"confidence"`       // 0.0-1.0
	LearnedAt   time.Time `json:"learned_at"`
}

// CodePattern represents a reusable code pattern.
type CodePattern struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Pattern     string            `json:"pattern"`
	Language    string            `json:"language,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	UsageCount  int               `json:"usage_count"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	LearnedAt   time.Time         `json:"learned_at"`
	LastUsed    time.Time         `json:"last_used"`
}

// ArchitectureDecisions stores project architecture info.
type ArchitectureDecisions struct {
	Style           string            `json:"style,omitempty"` // e.g., "microservices", "monolith", "modular"
	BuildSystem     string            `json:"build_system,omitempty"`
	TestFramework   string            `json:"test_framework,omitempty"`
	Directories     map[string]string `json:"directories,omitempty"` // key: purpose, value: path
	EntryPoints     []string          `json:"entry_points,omitempty"`
	Dependencies    []string          `json:"dependencies,omitempty"`
	CustomDecisions map[string]string `json:"custom_decisions,omitempty"`
}

// FilePattern tracks patterns for specific file types.
type FilePattern struct {
	Extension   string   `json:"extension"`
	Template    string   `json:"template,omitempty"`
	Conventions []string `json:"conventions,omitempty"`
	Directories []string `json:"directories,omitempty"` // Where these files usually live
}

// RecentFile tracks recently accessed files.
type RecentFile struct {
	Path       string    `json:"path"`
	AccessTime time.Time `json:"access_time"`
	EditCount  int       `json:"edit_count"`
}

// MemoryStore manages project memories across sessions.
type MemoryStore struct {
	dir      string
	mu       sync.RWMutex
	projects map[string]*ProjectMemory
	global   *GlobalMemory
}

// GlobalMemory stores cross-project preferences and patterns.
type GlobalMemory struct {
	UserPreferences map[string]string `json:"user_preferences,omitempty"`
	EditorSettings  map[string]string `json:"editor_settings,omitempty"`
	RecentProjects  []RecentProject   `json:"recent_projects,omitempty"`
	LearnedPatterns []CodePattern     `json:"learned_patterns,omitempty"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

// RecentProject tracks recently accessed projects.
type RecentProject struct {
	Path        string    `json:"path"`
	Name        string    `json:"name"`
	LastAccess  time.Time `json:"last_access"`
	AccessCount int       `json:"access_count"`
}

const (
	memoryDirName     = "memory"
	globalMemoryFile  = "global.json"
	projectMemoryFile = "project.json"
	maxRecentFiles    = 50
	maxRecentProjects = 20
)

// NewMemoryStore creates a new memory store.
func NewMemoryStore(dir string) (*MemoryStore, error) {
	memDir := filepath.Join(dir, memoryDirName)
	if err := os.MkdirAll(memDir, 0o700); err != nil {
		return nil, fmt.Errorf("memory: mkdir: %w", err)
	}
	ms := &MemoryStore{
		dir:      memDir,
		projects: make(map[string]*ProjectMemory),
		global:   &GlobalMemory{UpdatedAt: time.Now()},
	}
	_ = ms.loadGlobal()
	return ms, nil
}

// loadGlobal loads global memory from disk.
func (ms *MemoryStore) loadGlobal() error {
	path := filepath.Join(ms.dir, globalMemoryFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, ms.global)
}

// saveGlobal persists global memory to disk.
func (ms *MemoryStore) saveGlobal() error {
	ms.global.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(ms.global, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(ms.dir, globalMemoryFile)
	return atomicWriteFile(path, data, 0o600)
}

// projectDir returns the directory for a project's memory.
func (ms *MemoryStore) projectDir(projectPath string) string {
	// Create a safe directory name from project path
	safeName := strings.ReplaceAll(projectPath, string(filepath.Separator), "_")
	safeName = strings.ReplaceAll(safeName, ":", "_")
	safeName = strings.TrimPrefix(safeName, "_")
	return filepath.Join(ms.dir, "projects", safeName)
}

// GetProject retrieves or creates memory for a project.
func (ms *MemoryStore) GetProject(projectPath string) (*ProjectMemory, error) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if pm, ok := ms.projects[projectPath]; ok {
		pm.AccessCount++
		pm.UpdatedAt = time.Now()
		return pm, nil
	}

	// Try to load from disk
	projDir := ms.projectDir(projectPath)
	pmPath := filepath.Join(projDir, projectMemoryFile)

	var pm *ProjectMemory
	data, err := os.ReadFile(pmPath)
	if err == nil {
		pm = &ProjectMemory{}
		if err := json.Unmarshal(data, pm); err != nil {
			pm = nil
		}
	}

	if pm == nil {
		now := time.Now()
		pm = &ProjectMemory{
			ProjectPath:   projectPath,
			ProjectName:   filepath.Base(projectPath),
			Conventions:   make(map[string]Convention),
			CodePatterns:  []CodePattern{},
			TeamStandards: make(map[string]string),
			FilePatterns:  make(map[string]FilePattern),
			Preferences:   make(map[string]string),
			CreatedAt:     now,
			UpdatedAt:     now,
		}
	}

	pm.AccessCount++
	pm.UpdatedAt = time.Now()
	ms.projects[projectPath] = pm

	// Update recent projects in global memory
	ms.updateRecentProject(projectPath, pm.ProjectName)

	return pm, nil
}

// SaveProject persists a project's memory to disk.
func (ms *MemoryStore) SaveProject(pm *ProjectMemory) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	pm.UpdatedAt = time.Now()

	projDir := ms.projectDir(pm.ProjectPath)
	if err := os.MkdirAll(projDir, 0o700); err != nil {
		return fmt.Errorf("memory: mkdir project: %w", err)
	}

	data, err := json.MarshalIndent(pm, "", "  ")
	if err != nil {
		return fmt.Errorf("memory: marshal project: %w", err)
	}

	pmPath := filepath.Join(projDir, projectMemoryFile)
	if err := atomicWriteFile(pmPath, data, 0o600); err != nil {
		return fmt.Errorf("memory: write project: %w", err)
	}

	ms.projects[pm.ProjectPath] = pm
	return nil
}

// updateRecentProject adds or updates a project in recent list.
func (ms *MemoryStore) updateRecentProject(path, name string) {
	if ms.global.RecentProjects == nil {
		ms.global.RecentProjects = []RecentProject{}
	}

	// Check if already exists
	for i, rp := range ms.global.RecentProjects {
		if rp.Path == path {
			ms.global.RecentProjects[i].LastAccess = time.Now()
			ms.global.RecentProjects[i].AccessCount++
			// Move to front
			item := ms.global.RecentProjects[i]
			ms.global.RecentProjects = append(
				[]RecentProject{item},
				append(ms.global.RecentProjects[:i], ms.global.RecentProjects[i+1:]...)...,
			)
			return
		}
	}

	// Add new
	ms.global.RecentProjects = append([]RecentProject{{
		Path:        path,
		Name:        name,
		LastAccess:  time.Now(),
		AccessCount: 1,
	}}, ms.global.RecentProjects...)

	// Trim to max
	if len(ms.global.RecentProjects) > maxRecentProjects {
		ms.global.RecentProjects = ms.global.RecentProjects[:maxRecentProjects]
	}

	_ = ms.saveGlobal()
}

// LearnConvention adds or updates a convention for a project.
func (ms *MemoryStore) LearnConvention(projectPath, name, description string, confidence float64) error {
	pm, err := ms.GetProject(projectPath)
	if err != nil {
		return err
	}

	ms.mu.Lock()
	defer ms.mu.Unlock()

	if pm.Conventions == nil {
		pm.Conventions = make(map[string]Convention)
	}

	existing, exists := pm.Conventions[name]
	if exists {
		// Update confidence (moving average)
		existing.Confidence = (existing.Confidence + confidence) / 2
		existing.Description = description
		pm.Conventions[name] = existing
	} else {
		pm.Conventions[name] = Convention{
			Name:        name,
			Description: description,
			Confidence:  confidence,
			LearnedAt:   time.Now(),
		}
	}

	return ms.SaveProject(pm)
}

// LearnCodePattern adds a code pattern for a project.
func (ms *MemoryStore) LearnCodePattern(projectPath string, pattern CodePattern) error {
	pm, err := ms.GetProject(projectPath)
	if err != nil {
		return err
	}

	ms.mu.Lock()
	defer ms.mu.Unlock()

	pattern.ID = fmt.Sprintf("p_%d", time.Now().UnixNano())
	pattern.LearnedAt = time.Now()
	pattern.LastUsed = time.Now()

	pm.CodePatterns = append(pm.CodePatterns, pattern)
	return ms.SaveProject(pm)
}

// RecordFileAccess tracks file access for a project.
func (ms *MemoryStore) RecordFileAccess(projectPath, filePath string, edited bool) error {
	pm, err := ms.GetProject(projectPath)
	if err != nil {
		return err
	}

	ms.mu.Lock()
	defer ms.mu.Unlock()

	if pm.RecentFiles == nil {
		pm.RecentFiles = []RecentFile{}
	}

	// Check if file already tracked
	for i, rf := range pm.RecentFiles {
		if rf.Path == filePath {
			pm.RecentFiles[i].AccessTime = time.Now()
			if edited {
				pm.RecentFiles[i].EditCount++
			}
			// Move to front
			item := pm.RecentFiles[i]
			pm.RecentFiles = append(
				[]RecentFile{item},
				append(pm.RecentFiles[:i], pm.RecentFiles[i+1:]...)...,
			)
			return nil
		}
	}

	// Add new
	editCount := 0
	if edited {
		editCount = 1
	}
	pm.RecentFiles = append([]RecentFile{{
		Path:       filePath,
		AccessTime: time.Now(),
		EditCount:  editCount,
	}}, pm.RecentFiles...)

	// Trim to max
	if len(pm.RecentFiles) > maxRecentFiles {
		pm.RecentFiles = pm.RecentFiles[:maxRecentFiles]
	}

	return nil
}

// SetArchitecture sets architecture decisions for a project.
func (ms *MemoryStore) SetArchitecture(projectPath string, arch *ArchitectureDecisions) error {
	pm, err := ms.GetProject(projectPath)
	if err != nil {
		return err
	}

	ms.mu.Lock()
	defer ms.mu.Unlock()

	pm.Architecture = arch
	return ms.SaveProject(pm)
}

// GetRecentProjects returns recently accessed projects.
func (ms *MemoryStore) GetRecentProjects(limit int) []RecentProject {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	if ms.global.RecentProjects == nil {
		return nil
	}

	if limit <= 0 || limit > len(ms.global.RecentProjects) {
		limit = len(ms.global.RecentProjects)
	}

	result := make([]RecentProject, limit)
	copy(result, ms.global.RecentProjects[:limit])
	return result
}

// SetUserPreference sets a global user preference.
func (ms *MemoryStore) SetUserPreference(key, value string) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if ms.global.UserPreferences == nil {
		ms.global.UserPreferences = make(map[string]string)
	}
	ms.global.UserPreferences[key] = value
	return ms.saveGlobal()
}

// GetUserPreference retrieves a global user preference.
func (ms *MemoryStore) GetUserPreference(key string) (string, bool) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	if ms.global.UserPreferences == nil {
		return "", false
	}
	v, ok := ms.global.UserPreferences[key]
	return v, ok
}

// GetMostUsedPatterns returns the most frequently used patterns.
func (ms *MemoryStore) GetMostUsedPatterns(projectPath string, limit int) []CodePattern {
	pm, err := ms.GetProject(projectPath)
	if err != nil || pm == nil {
		return nil
	}

	ms.mu.RLock()
	defer ms.mu.RUnlock()

	patterns := make([]CodePattern, len(pm.CodePatterns))
	copy(patterns, pm.CodePatterns)

	sort.Slice(patterns, func(i, j int) bool {
		return patterns[i].UsageCount > patterns[j].UsageCount
	})

	if limit > 0 && limit < len(patterns) {
		patterns = patterns[:limit]
	}
	return patterns
}

// IncrementPatternUsage increments usage count for a pattern.
func (ms *MemoryStore) IncrementPatternUsage(projectPath, patternID string) error {
	pm, err := ms.GetProject(projectPath)
	if err != nil {
		return err
	}

	ms.mu.Lock()
	defer ms.mu.Unlock()

	for i, p := range pm.CodePatterns {
		if p.ID == patternID {
			pm.CodePatterns[i].UsageCount++
			pm.CodePatterns[i].LastUsed = time.Now()
			break
		}
	}

	return ms.SaveProject(pm)
}

// SaveAll persists all loaded project memories.
func (ms *MemoryStore) SaveAll() error {
	ms.mu.RLock()
	projects := make([]*ProjectMemory, 0, len(ms.projects))
	for _, pm := range ms.projects {
		projects = append(projects, pm)
	}
	ms.mu.RUnlock()

	var lastErr error
	for _, pm := range projects {
		if err := ms.SaveProject(pm); err != nil {
			lastErr = err
		}
	}

	if err := ms.saveGlobal(); err != nil {
		lastErr = err
	}

	return lastErr
}
