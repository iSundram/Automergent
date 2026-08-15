package context

import (
	"context"
	"os"
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
	Hash         string         `json:"hash,omitempty"`
	NeedsRefresh bool           `json:"needs_refresh"`
}

// StalenessDetector tracks file freshness and cache validity.
type StalenessDetector struct {
	mu         sync.RWMutex
	cache      map[string]*FileStatus
	repoRoot   string
	staleAfter time.Duration
}

// StalenessConfig configures the staleness detector.
type StalenessConfig struct {
	StaleAfter time.Duration
}

// DefaultStalenessConfig returns default configuration.
func DefaultStalenessConfig() StalenessConfig {
	return StalenessConfig{
		StaleAfter: 5 * time.Minute,
	}
}

// NewStalenessDetector creates a new staleness detector.
func NewStalenessDetector(repoRoot string, cfg StalenessConfig) *StalenessDetector {
	return &StalenessDetector{
		cache:      make(map[string]*FileStatus),
		repoRoot:   repoRoot,
		staleAfter: cfg.StaleAfter,
	}
}

// Check returns the current staleness status of a file.
func (sd *StalenessDetector) Check(ctx context.Context, path string) (*FileStatus, error) {
	sd.mu.RLock()
	cached, exists := sd.cache[path]
	sd.mu.RUnlock()

	// Get current file info
	info, err := osStat(path)
	if err != nil {
		if isNotExist(err) {
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

	sd.updateCache(path, status)
	return status, nil
}

// CheckBatch checks multiple files efficiently.
func (sd *StalenessDetector) CheckBatch(ctx context.Context, paths []string) (map[string]*FileStatus, error) {
	results := make(map[string]*FileStatus, len(paths))
	var wg sync.WaitGroup
	var mu sync.Mutex
	errCh := make(chan error, 1)

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
	}
}

// InvalidateAll marks all files as needing refresh.
func (sd *StalenessDetector) InvalidateAll() {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	for _, status := range sd.cache {
		status.NeedsRefresh = true
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

// WatchForChanges starts watching for file changes (blocking).
func (sd *StalenessDetector) WatchForChanges(ctx context.Context, onChange func(path string)) error {
	ticker := time.NewTicker(30 * time.Second)
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

// StartAutoRefresh starts automatic refresh in the background.
func (cr *CacheRefresher) StartAutoRefresh(ctx context.Context, interval time.Duration) {
	if cr.strategy != RefreshProactive {
		return
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = cr.RefreshStale(ctx)
			}
		}
	}()
}

// updateCache updates the cache with a new status.
func (sd *StalenessDetector) updateCache(path string, status *FileStatus) {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	sd.cache[path] = status
}

// osStat is an alias for os.Stat for testing.
func osStat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

// isNotExist checks if an error is a file not exists error.
func isNotExist(err error) bool {
	return os.IsNotExist(err)
}
