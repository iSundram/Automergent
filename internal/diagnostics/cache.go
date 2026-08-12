package diagnostics

import (
	"sync"
	"time"
)

type cacheEntry struct {
	diags   []Diagnostic
	expires time.Time
}

var (
	cacheMu sync.Mutex
	cache   = make(map[string]*cacheEntry)
)

// cacheKey produces a deterministic key from path and content.
func cacheKey(path, content string) string {
	return path + "\x00" + content
}

func cacheGet(key string) ([]Diagnostic, bool) {
	cacheMu.Lock()
	defer cacheMu.Unlock()

	entry, ok := cache[key]
	if !ok || time.Now().After(entry.expires) {
		delete(cache, key)
		return nil, false
	}
	return entry.diags, true
}

func cachePut(key string, diags []Diagnostic) {
	cfg := loadCacheDuration()

	cacheMu.Lock()
	defer cacheMu.Unlock()

	cache[key] = &cacheEntry{
		diags:   diags,
		expires: time.Now().Add(cfg),
	}
}

// loadCacheDuration reads the configured TTL from config.
func loadCacheDuration() time.Duration {
	from := loadConfig().Diagnostics.CacheDurationSec
	if from <= 0 {
		from = 30
	}
	return time.Duration(from) * time.Second
}
