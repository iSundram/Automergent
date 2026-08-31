// Package cache provides a persistent disk-backed cache for diagnostics.
package cache

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/iSundram/Automergent/internal/diagnostics/types"
	_ "modernc.org/sqlite"
)

// PersistentCache is a SQLite-backed cache for diagnostics.
type PersistentCache struct {
	db       *sql.DB
	mu       sync.RWMutex
	cacheDir string
	enabled  bool
	done     chan struct{}
}

// CacheEntry represents a cached diagnostic result.
type CacheEntry struct {
	Key       string    `json:"key"`
	FilePath  string    `json:"file_path"`
	ContentHash string  `json:"content_hash"`
	Language  string    `json:"language"`
	Diagnostics []types.Diagnostic `json:"diagnostics"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	HitCount  int       `json:"hit_count"`
}

// NewPersistentCache creates a new persistent cache.
func NewPersistentCache(cacheDir string, maxAge time.Duration) (*PersistentCache, error) {
	if cacheDir == "" {
		home, _ := os.UserHomeDir()
		cacheDir = filepath.Join(home, ".automergent", "diagnostics_cache")
	}

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("create cache dir: %w", err)
	}

	dbPath := filepath.Join(cacheDir, "diagnostics.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Enable WAL mode for better concurrency
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}

	pc := &PersistentCache{
		db:       db,
		cacheDir: cacheDir,
		enabled:  true,
		done:     make(chan struct{}),
	}

	if err := pc.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}

	// Start cleanup goroutine
	go pc.cleanupLoop(maxAge)

	return pc, nil
}

func (pc *PersistentCache) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS cache_entries (
		key TEXT PRIMARY KEY,
		file_path TEXT NOT NULL,
		content_hash TEXT NOT NULL,
		language TEXT NOT NULL,
		diagnostics TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		expires_at DATETIME NOT NULL,
		hit_count INTEGER DEFAULT 0
	);
	CREATE INDEX IF NOT EXISTS idx_expires_at ON cache_entries(expires_at);
	CREATE INDEX IF NOT EXISTS idx_file_path ON cache_entries(file_path);
	CREATE INDEX IF NOT EXISTS idx_content_hash ON cache_entries(content_hash);
	`
	_, err := pc.db.Exec(schema)
	return err
}

// contentHash computes a hash of the content for cache key.
func contentHash(content string) string {
	h := fnv.New64a()
	h.Write([]byte(content))
	return fmt.Sprintf("%x", h.Sum64())
}

// cacheKey generates a deterministic key from path and content hash.
func cacheKey(path, contentHash string) string {
	return path + "|" + contentHash
}

// Get retrieves cached diagnostics if valid.
func (pc *PersistentCache) Get(path, content string, maxAge time.Duration) ([]types.Diagnostic, bool) {
	if !pc.enabled {
		return nil, false
	}

	hash := contentHash(content)
	key := cacheKey(path, hash)

	pc.mu.RLock()
	defer pc.mu.RUnlock()

	var diagnosticsJSON string
	var expiresAt time.Time
	var hitCount int

	err := pc.db.QueryRow(
		`SELECT diagnostics, expires_at, hit_count FROM cache_entries WHERE key = ?`,
		key,
	).Scan(&diagnosticsJSON, &expiresAt, &hitCount)

	if err == sql.ErrNoRows {
		return nil, false
	}
	if err != nil {
		return nil, false
	}

	if time.Now().After(expiresAt) {
		// Expired - delete and return miss
		go pc.Delete(key)
		return nil, false
	}

	var diags []types.Diagnostic
	if err := json.Unmarshal([]byte(diagnosticsJSON), &diags); err != nil {
		return nil, false
	}

	// Update hit count asynchronously; the SQL-side increment avoids the
	// read-modify-write race between concurrent Gets.
	go pc.incrementHitCount(key)

	return diags, true
}

// Put stores diagnostics in the cache. Clean results (no diagnostics) are
// cached too — they are just as expensive to compute and just as reusable.
func (pc *PersistentCache) Put(path, content, language string, diags []types.Diagnostic, ttl time.Duration) error {
	if !pc.enabled {
		return nil
	}

	hash := contentHash(content)
	key := cacheKey(path, hash)
	now := time.Now()
	expiresAt := now.Add(ttl)

	diagnosticsJSON, err := json.Marshal(diags)
	if err != nil {
		return err
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	_, err = pc.db.Exec(`
		INSERT OR REPLACE INTO cache_entries (key, file_path, content_hash, language, diagnostics, created_at, expires_at, hit_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, 0)
	`, key, path, hash, language, string(diagnosticsJSON), now, expiresAt)

	return err
}

// Delete removes a cache entry.
func (pc *PersistentCache) Delete(key string) error {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	_, err := pc.db.Exec(`DELETE FROM cache_entries WHERE key = ?`, key)
	return err
}

// DeleteByPath removes all cache entries for a file path.
func (pc *PersistentCache) DeleteByPath(path string) error {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	_, err := pc.db.Exec(`DELETE FROM cache_entries WHERE file_path = ?`, path)
	return err
}

// incrementHitCount updates the hit count for a cache entry.
func (pc *PersistentCache) incrementHitCount(key string) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.db.Exec(`UPDATE cache_entries SET hit_count = hit_count + 1 WHERE key = ?`, key)
}

// cleanupLoop periodically removes expired entries.
func (pc *PersistentCache) cleanupLoop(maxAge time.Duration) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-pc.done:
			return
		case <-ticker.C:
			if !pc.enabled {
				return
			}
			pc.mu.Lock()
			cutoff := time.Now().Add(-maxAge)
			_, err := pc.db.Exec(`DELETE FROM cache_entries WHERE expires_at < ? OR created_at < ?`, time.Now(), cutoff)
			pc.mu.Unlock()
			if err != nil {
				// Log error but continue
			}
		}
	}
}

// Stats returns cache statistics.
func (pc *PersistentCache) Stats() (CacheStats, error) {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	var stats CacheStats
	err := pc.db.QueryRow(`SELECT COUNT(*), SUM(hit_count) FROM cache_entries`).Scan(&stats.EntryCount, &stats.TotalHits)
	if err != nil {
		return stats, err
	}

	var size int64
	err = pc.db.QueryRow(`SELECT page_count * page_size FROM pragma_page_count, pragma_page_size`).Scan(&size)
	stats.SizeBytes = size

	return stats, nil
}

// CacheStats holds cache statistics.
type CacheStats struct {
	EntryCount int   `json:"entry_count"`
	TotalHits  int   `json:"total_hits"`
	SizeBytes  int64 `json:"size_bytes"`
}

// Close closes the cache database and stops the cleanup goroutine.
func (pc *PersistentCache) Close() error {
	pc.enabled = false
	select {
	case <-pc.done:
		// already closed
	default:
		close(pc.done)
	}
	return pc.db.Close()
}

// Clear removes all cache entries.
func (pc *PersistentCache) Clear() error {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	_, err := pc.db.Exec(`DELETE FROM cache_entries`)
	return err
}

// GetByLanguage returns cache entries for a specific language.
func (pc *PersistentCache) GetByLanguage(language string, limit int) ([]CacheEntry, error) {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	rows, err := pc.db.Query(`
		SELECT key, file_path, content_hash, language, diagnostics, created_at, expires_at, hit_count
		FROM cache_entries WHERE language = ? ORDER BY hit_count DESC LIMIT ?
	`, language, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []CacheEntry
	for rows.Next() {
		var e CacheEntry
		var diagsJSON string
		if err := rows.Scan(&e.Key, &e.FilePath, &e.ContentHash, &e.Language, &diagsJSON, &e.CreatedAt, &e.ExpiresAt, &e.HitCount); err != nil {
			continue
		}
		json.Unmarshal([]byte(diagsJSON), &e.Diagnostics)
		entries = append(entries, e)
	}
	return entries, nil
}

// Vacuum rebuilds the database to reclaim space.
func (pc *PersistentCache) Vacuum() error {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	_, err := pc.db.Exec(`VACUUM`)
	return err
}

// Global cache instance
var globalCache *PersistentCache
var globalCacheOnce sync.Once

// GetGlobalCache returns the global persistent cache instance.
func GetGlobalCache() *PersistentCache {
	return globalCache
}

// InitGlobalCache initializes the global cache.
func InitGlobalCache(cacheDir string, maxAge time.Duration) error {
	var err error
	globalCacheOnce.Do(func() {
		globalCache, err = NewPersistentCache(cacheDir, maxAge)
	})
	return err
}