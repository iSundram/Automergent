package context

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// StalenessState represents the freshness state of a file.
type StalenessState int

const (
	StateFresh StalenessState = iota
	StateModified
	StateStale
	StateInvalid
)

// String returns a human-readable state name.
func (s StalenessState) String() string {
	switch s {
	case StateFresh:
		return "fresh"
	case StateModified:
		return "modified"
	case StateStale:
		return "stale"
	case StateInvalid:
		return "invalid"
	default:
		return "unknown"
	}
}

// FileStatus holds the staleness status of a file.
type FileStatus struct {
	Path         string         `json:"path"`
	State        StalenessState `json:"state"`
	ModTime      time.Time      `json:"mod_time"`
	CachedTime   time.Time      `json:"cached_time"`
	GitStatus    GitFileStatus  `json:"git_status,omitempty"`
	Hash         string         `json:"hash,omitempty"`
	NeedsRefresh bool           `json:"needs_refresh"`
}

// GitFileStatus represents git status for a file.
type GitFileStatus struct {
	IsTracked   bool   `json:"is_tracked"`
	HasChanges  bool   `json:"has_changes"`
	IsStaged    bool   `json:"is_staged"`
	IsNew       bool   `json:"is_new"`
	IsDeleted   bool   `json:"is_deleted"`
	DiffSummary string `json:"diff_summary,omitempty"`
}

// StalenessDetector tracks file freshness and cache validity.
type StalenessDetector struct {
	mu               sync.RWMutex
	cache            map[string]*FileStatus
	repoRoot         string
	staleAfter       time.Duration
	gitAware         bool
	lastGitCheck     time.Time
	gitCheckInterval time.Duration
}

// StalenessConfig configures the staleness detector.
type StalenessConfig struct {
	StaleAfter       time.Duration
	GitAware         bool
	GitCheckInterval time.Duration
}

// DefaultStalenessConfig returns default configuration.
func DefaultStalenessConfig() StalenessConfig {
	return StalenessConfig{
		StaleAfter:       5 * time.Minute,
		GitAware:         true,
		GitCheckInterval: 30 * time.Second,
	}
}

// NewStalenessDetector creates a new staleness detector.
func NewStalenessDetector(repoRoot string, cfg StalenessConfig) *StalenessDetector {
	return &StalenessDetector{
		cache:            make(map[string]*FileStatus),
		repoRoot:         repoRoot,
		staleAfter:       cfg.StaleAfter,
		gitAware:         cfg.GitAware,
		gitCheckInterval: cfg.GitCheckInterval,
	}
}

// Check returns the current staleness status of a file.
func (sd *StalenessDetector) Check(ctx context.Context, path string) (*FileStatus, error) {
	sd.mu.RLock()
	cached, exists := sd.cache[path]
	sd.mu.RUnlock()

	// Get current file info
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			status := &FileStatus{
				Path:         path,
				State:        StateInvalid,
				NeedsRefresh: true,
			}
			sd.updateCache(path, status)
			return status, nil
		}
		return nil, err
	}

	modTime := info.ModTime()

	// Check if cached entry is still valid
	if exists && cached.ModTime.Equal(modTime) && time.Since(cached.CachedTime) < sd.staleAfter {
		return cached, nil
	}

	// Need to re-evaluate
	status := &FileStatus{
		Path:       path,
		ModTime:    modTime,
		CachedTime: time.Now(),
	}

	// Determine state based on modification time
	if exists && cached.ModTime.Before(modTime) {
		status.State = StateModified
		status.NeedsRefresh = true
	} else if time.Since(modTime) > sd.staleAfter {
		status.State = StateStale
	} else {
		status.State = StateFresh
	}

	// Add git awareness
	if sd.gitAware {
		gitStatus, err := sd.getGitStatus(ctx, path)
		if err == nil {
			status.GitStatus = gitStatus
			if gitStatus.HasChanges {
				status.State = StateModified
				status.NeedsRefresh = true
			}
		}
	}

	sd.updateCache(path, status)
	return status, nil
}

// CheckBatch checks multiple files efficiently.
func (sd *StalenessDetector) CheckBatch(ctx context.Context, paths []string) (map[string]*FileStatus, error) {
	results := make(map[string]*FileStatus, len(paths))
	var wg sync.WaitGroup
	var mu sync.Mutex
	errCh := make(chan error, 1)

	// Refresh git status for all files in batch (protected read of lastGitCheck)
	if sd.gitAware {
		sd.mu.RLock()
		last := sd.lastGitCheck
		sd.mu.RUnlock()
		if time.Since(last) > sd.gitCheckInterval {
			if err := sd.refreshGitStatus(ctx); err != nil {
				// Non-fatal, continue without git info
			}
		}
	}

	for _, path := range paths {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			status, err := sd.Check(ctx, p)
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
				return
			}
			mu.Lock()
			results[p] = status
			mu.Unlock()
		}(path)
	}

	wg.Wait()

	select {
	case err := <-errCh:
		return results, err
	default:
		return results, nil
	}
}

// GetStaleFiles returns all files that need refreshing.
func (sd *StalenessDetector) GetStaleFiles() []string {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	var stale []string
	for path, status := range sd.cache {
		if status.NeedsRefresh || status.State == StateStale || status.State == StateModified {
			stale = append(stale, path)
		}
	}
	return stale
}

// MarkFresh marks a file as freshly cached.
func (sd *StalenessDetector) MarkFresh(path string) {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	if status, exists := sd.cache[path]; exists {
		status.State = StateFresh
		status.NeedsRefresh = false
		status.CachedTime = time.Now()
	}
}

// MarkAllFresh marks all cached files as fresh.
func (sd *StalenessDetector) MarkAllFresh() {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	for _, status := range sd.cache {
		status.State = StateFresh
		status.NeedsRefresh = false
		status.CachedTime = time.Now()
	}
}

// Invalidate marks a file as needing refresh.
func (sd *StalenessDetector) Invalidate(path string) {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	if status, exists := sd.cache[path]; exists {
		status.NeedsRefresh = true
		status.State = StateStale
	}
}

// InvalidateAll marks all files as needing refresh.
func (sd *StalenessDetector) InvalidateAll() {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	for _, status := range sd.cache {
		status.NeedsRefresh = true
		status.State = StateStale
	}
}

// Remove removes a file from the cache.
func (sd *StalenessDetector) Remove(path string) {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	delete(sd.cache, path)
}

// Clear clears the entire cache.
func (sd *StalenessDetector) Clear() {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	sd.cache = make(map[string]*FileStatus)
}

// updateCache safely updates the cache.
func (sd *StalenessDetector) updateCache(path string, status *FileStatus) {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	sd.cache[path] = status
}

// getGitStatus gets git status for a single file.
func (sd *StalenessDetector) getGitStatus(ctx context.Context, path string) (GitFileStatus, error) {
	status := GitFileStatus{}

	// Make path relative to repo root
	relPath := path
	if sd.repoRoot != "" {
		var err error
		relPath, err = filepath.Rel(sd.repoRoot, path)
		if err != nil {
			relPath = path
		}
	}

	// Check if file is tracked (limit output to avoid OOM)
	if _, err := runCmdLimited(exec.CommandContext(ctx, "git", "-C", sd.repoRoot, "ls-files", "--error-unmatch", relPath), 1<<20); err != nil {
		// File is not tracked
		status.IsTracked = false
		status.IsNew = true
		return status, nil
	}
	status.IsTracked = true

	// Check for changes
	if out, err := runCmdLimited(exec.CommandContext(ctx, "git", "-C", sd.repoRoot, "diff", "--name-only", relPath), 1<<20); err == nil {
		if len(strings.TrimSpace(string(out))) > 0 {
			status.HasChanges = true
		}
	}

	// Check for staged changes
	if out, err := runCmdLimited(exec.CommandContext(ctx, "git", "-C", sd.repoRoot, "diff", "--cached", "--name-only", relPath), 1<<20); err == nil {
		if len(strings.TrimSpace(string(out))) > 0 {
			status.IsStaged = true
			status.HasChanges = true
		}
	}

	return status, nil
}

// refreshGitStatus refreshes git status for all tracked files.
func (sd *StalenessDetector) refreshGitStatus(ctx context.Context) error {
	sd.mu.Lock()
	sd.lastGitCheck = time.Now()
	sd.mu.Unlock()

	// Get all changed files (limit output size)
	out, err := runCmdLimited(exec.CommandContext(ctx, "git", "-C", sd.repoRoot, "status", "--porcelain", "--untracked-files=all"), 2<<20)
	if err != nil {
		return err
	}

	// Parse status output
	changedFiles := make(map[string]GitFileStatus)
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if len(line) < 4 {
			continue
		}
		// Status format: XY PATH
		status := line[:2]
		path := strings.TrimSpace(line[3:])
		if path == "" {
			continue
		}

		fullPath := filepath.Join(sd.repoRoot, path)
		gitStatus := GitFileStatus{IsTracked: true}

		// Parse status codes
		switch {
		case status[0] == '?' || status[1] == '?':
			gitStatus.IsNew = true
			gitStatus.IsTracked = false
		case status[0] == 'D' || status[1] == 'D':
			gitStatus.IsDeleted = true
			gitStatus.HasChanges = true
		case status[0] != ' ':
			gitStatus.IsStaged = true
			gitStatus.HasChanges = true
		case status[1] != ' ':
			gitStatus.HasChanges = true
		}

		changedFiles[fullPath] = gitStatus
	}

	// Update cache with git status
	sd.mu.Lock()
	for path, gitStatus := range changedFiles {
		if cached, exists := sd.cache[path]; exists {
			cached.GitStatus = gitStatus
			if gitStatus.HasChanges {
				cached.State = StateModified
				cached.NeedsRefresh = true
			}
		}
	}
	sd.mu.Unlock()

	return nil
}

// runCmdLimited runs a command and limits the amount of stdout read to maxBytes.
func runCmdLimited(cmd *exec.Cmd, maxBytes int64) ([]byte, error) {
	outPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	lr := io.LimitReader(outPipe, maxBytes+1)
	data, _ := io.ReadAll(lr)
	err = cmd.Wait()
	if int64(len(data)) > maxBytes {
		return data[:maxBytes], fmt.Errorf("command output truncated")
	}
	return data, err
}

// GetModifiedFiles returns all files with uncommitted changes.
func (sd *StalenessDetector) GetModifiedFiles(ctx context.Context) ([]string, error) {
	if err := sd.refreshGitStatus(ctx); err != nil {
		return nil, err
	}

	sd.mu.RLock()
	defer sd.mu.RUnlock()

	var modified []string
	for path, status := range sd.cache {
		if status.GitStatus.HasChanges || status.GitStatus.IsNew {
			modified = append(modified, path)
		}
	}
	return modified, nil
}

// WatchForChanges starts watching for file changes (blocking).
func (sd *StalenessDetector) WatchForChanges(ctx context.Context, onChange func(path string)) error {
	// Simple polling implementation
	ticker := time.NewTicker(sd.gitCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			stale := sd.GetStaleFiles()
			for _, path := range stale {
				onChange(path)
			}
		}
	}
}

// RefreshStrategy defines how to refresh stale files.
type RefreshStrategy int

const (
	RefreshOnDemand RefreshStrategy = iota
	RefreshProactive
	RefreshLazy
)

// CacheRefresher handles automatic cache refresh.
type CacheRefresher struct {
	detector   *StalenessDetector
	strategy   RefreshStrategy
	refreshFn  func(path string) error
	mu         sync.RWMutex
	refreshing map[string]bool
}

// NewCacheRefresher creates a new cache refresher.
func NewCacheRefresher(detector *StalenessDetector, strategy RefreshStrategy) *CacheRefresher {
	return &CacheRefresher{
		detector:   detector,
		strategy:   strategy,
		refreshing: make(map[string]bool),
	}
}

// SetRefreshFn sets the function to call when refreshing a file.
func (cr *CacheRefresher) SetRefreshFn(fn func(path string) error) {
	cr.refreshFn = fn
}

// RefreshStale refreshes all stale files.
func (cr *CacheRefresher) RefreshStale(ctx context.Context) error {
	stale := cr.detector.GetStaleFiles()

	var wg sync.WaitGroup
	errCh := make(chan error, 1)

	for _, path := range stale {
		// Check if already refreshing
		cr.mu.RLock()
		if cr.refreshing[path] {
			cr.mu.RUnlock()
			continue
		}
		cr.mu.RUnlock()

		wg.Add(1)
		go func(p string) {
			defer wg.Done()

			cr.mu.Lock()
			cr.refreshing[p] = true
			cr.mu.Unlock()

			defer func() {
				cr.mu.Lock()
				delete(cr.refreshing, p)
				cr.mu.Unlock()
			}()

			if cr.refreshFn != nil {
				if err := cr.refreshFn(p); err != nil {
					select {
					case errCh <- err:
					default:
					}
					return
				}
			}
			cr.detector.MarkFresh(p)
		}(path)
	}

	wg.Wait()

	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

// StartAutoRefresh starts automatic background refresh.
func (cr *CacheRefresher) StartAutoRefresh(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if cr.strategy == RefreshProactive {
					_ = cr.RefreshStale(ctx)
				}
			}
		}
	}()
}
