package diagnostics

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// maxCacheEntries bounds the in-memory analysis cache. Without a bound the
// map grows with every distinct file version ever analyzed.
const maxCacheEntries = 256

type cacheEntry struct {
	key     string
	diags   []Diagnostic
	expires time.Time
}

var (
	cacheMu sync.Mutex
	cache   = make(map[string]*list.Element)
	// lru is the eviction order: front = most recent, back = evict first.
	lru list.List
)

// cacheKey produces a deterministic key from path and a content digest. The
// content itself is never stored in the key, so large files do not stay
// resident in the cache map.
func cacheKey(path, content string) string {
	sum := sha256.Sum256([]byte(content))
	return path + "\x00" + hex.EncodeToString(sum[:])
}

func cacheGet(key string) ([]Diagnostic, bool) {
	cacheMu.Lock()
	defer cacheMu.Unlock()

	el, ok := cache[key]
	if !ok {
		return nil, false
	}
	entry := el.Value.(*cacheEntry)
	if time.Now().After(entry.expires) {
		lru.Remove(el)
		delete(cache, key)
		return nil, false
	}
	lru.MoveToFront(el)
	// Return a copy: callers may append to or sort the result, and mutating
	// the cached slice would corrupt future hits.
	out := make([]Diagnostic, len(entry.diags))
	copy(out, entry.diags)
	return out, true
}

func cachePut(key string, diags []Diagnostic) {
	cfg := loadCacheDuration()

	cacheMu.Lock()
	defer cacheMu.Unlock()

	if el, ok := cache[key]; ok {
		entry := el.Value.(*cacheEntry)
		entry.diags = diags
		entry.expires = time.Now().Add(cfg)
		lru.MoveToFront(el)
		return
	}

	entry := &cacheEntry{
		key:     key,
		diags:   diags,
		expires: time.Now().Add(cfg),
	}
	cache[key] = lru.PushFront(entry)
	for len(cache) > maxCacheEntries {
		oldest := lru.Back()
		if oldest == nil {
			break
		}
		lru.Remove(oldest)
		delete(cache, oldest.Value.(*cacheEntry).key)
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
