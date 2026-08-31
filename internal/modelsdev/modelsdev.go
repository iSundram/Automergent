// Package modelsdev loads the community model catalog from models.dev
// (https://models.dev/api.json). The catalog gives every model its display
// name, context/output limits, pricing (including cache pricing), reasoning
// capability and knowledge cutoff without a live provider API call.
//
// The raw catalog JSON is cached once on disk and filtered per provider at
// read time, so a single fetch serves every provider. Resolution order:
//
//  1. disk cache (~/.automergent/cache/models-dev.json) while fresh,
//  2. the network (written atomically to that cache),
//  3. the stale disk cache when the network is unreachable,
//  4. the embedded snapshot as the final fallback.
//
// Filters applied to every list (user-registered custom models bypass them):
// models must support tool calling and a context window of at least
// MinContextTokens — the agent loop needs long context.
//
// The cache path can be overridden with AUTOMERGENT_MODELS_DEV_PATH (tests)
// and the network fetch disabled with AUTOMERGENT_MODELS_DEV_OFFLINE=1.
package modelsdev

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/iSundram/Automergent/internal/ai"
)

const (
	sourceURL    = "https://models.dev/api.json"
	cacheTTL     = 5 * time.Minute
	refreshEvery = time.Hour
)

// MinContextTokens is the minimum context window a catalog model needs to be
// listed. The agent's conversation loop (system prompt, tools, artifacts,
// compaction headroom) only fits comfortably in a 1M-token window.
const MinContextTokens = 1_000_000

var (
	mu        sync.Mutex
	cached    map[string][]ai.Model // provider slug -> models
	cachedAt  time.Time
	cachePath string
)

// providerSlugs maps Automergent provider names to models.dev provider keys.
// Legacy names resolve through the same map; unknown providers (custom
// endpoints) have no catalog entry and get an empty list.
var providerSlugs = map[string]string{
	"google-aistudio": "google",
	"google-vertex":   "google",
	"google":          "google",
	"anthropic":       "anthropic",
	"openai":          "openai",
	"deepseek":        "deepseek",
	"ollama":          "ollama-cloud",
}

// SlugFor resolves a provider name to its models.dev provider key. The second
// return is false when the provider has no catalog counterpart (custom
// endpoints), in which case callers fall back to their own listing.
func SlugFor(provider string) (string, bool) {
	slug, ok := providerSlugs[provider]
	return slug, ok
}

// Models returns the catalog's model list for a provider, filtered to
// tool-call capable models with at least MinContextTokens of context. It
// never fails: every fallback ends at the embedded snapshot, so callers get
// a usable list even fully offline on first run.
func Models(ctx context.Context, provider string) []ai.Model {
	slug, ok := SlugFor(provider)
	if !ok {
		return nil
	}
	models := catalog(ctx)[slug]
	return append([]ai.Model{}, models...)
}

// ModelInfo returns the full catalog entry for one model. It answers
// "tell me about this model" (/model info) from the same cache as Models.
func ModelInfo(ctx context.Context, provider, id string) (ai.Model, bool) {
	for _, m := range Models(ctx, provider) {
		if m.ID == id {
			return m, true
		}
	}
	return ai.Model{}, false
}

// Refresh forces a network refresh (backs /model refresh and the hourly
// background refresh). Failure is silent: the existing cache stays valid.
func Refresh(ctx context.Context) {
	mu.Lock()
	defer mu.Unlock()
	fetchCatalogLocked(ctx)
}

// StartRefresher runs the periodic background refresh (hourly) until ctx is
// cancelled.
func StartRefresher(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(refreshEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				Refresh(ctx)
			}
		}
	}()
}

// catalog returns the memoized per-provider model map, refreshing from disk
// or network when the in-memory copy is stale.
func catalog(ctx context.Context) map[string][]ai.Model {
	mu.Lock()
	defer mu.Unlock()
	if cached != nil && time.Since(cachedAt) < cacheTTL {
		return cached
	}
	models := loadCachedFile()
	if models != nil {
		cached, cachedAt = models, time.Now()
		return cached
	}
	if fetchCatalogLocked(ctx) {
		return cached
	}
	// Network unavailable: keep the embedded snapshot. cached stays nil so a
	// later call can retry when connectivity returns.
	return snapshotCatalog
}

// cacheFile resolves the cache location, memoized.
func cacheFile() string {
	if cachePath != "" {
		return cachePath
	}
	dir := os.Getenv("AUTOMERGENT_MODELS_DEV_PATH")
	if dir != "" {
		cachePath = dir
		return cachePath
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		cachePath = filepath.Join(os.TempDir(), "automergent-models-dev.json")
		return cachePath
	}
	cachePath = filepath.Join(home, ".automergent", "cache", "models-dev.json")
	return cachePath
}

// loadCachedFile reads the disk cache. A fresh mtime is not required here:
// staleness is acceptable when the network is down; only the in-memory copy
// obeys the strict TTL.
func loadCachedFile() map[string][]ai.Model {
	data, err := os.ReadFile(cacheFile())
	if err != nil {
		return nil
	}
	return parseCatalog(data)
}

// catalogProvider is the subset of the catalog schema we consume.
type catalogProvider struct {
	Models map[string]catalogModel `json:"models"`
}

type catalogModel struct {
	Name        string `json:"name"`
	Reasoning   bool   `json:"reasoning"`
	Attachment  bool   `json:"attachment"`
	ToolCall    bool   `json:"tool_call"`
	Knowledge   string `json:"knowledge"`
	ReleaseDate string `json:"release_date"`
	Limit       struct {
		Context int `json:"context"`
		Output  int `json:"output"`
	} `json:"limit"`
	Cost struct {
		Input      float64 `json:"input"`
		Output     float64 `json:"output"`
		CacheRead  float64 `json:"cache_read"`
		CacheWrite float64 `json:"cache_write"`
	} `json:"cost"`
	ReasoningOptions []struct {
		Type   string   `json:"type"`
		Values []string `json:"values"`
	} `json:"reasoning_options"`
}

// parseCatalog extracts every provider's tool-call capable, long-context
// models. Providers with no qualifying models are omitted.
func parseCatalog(data []byte) map[string][]ai.Model {
	var catalog map[string]catalogProvider
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil
	}
	out := make(map[string][]ai.Model)
	for slug, prov := range catalog {
		var models []ai.Model
		for id, m := range prov.Models {
			if !passesFilter(m) {
				continue
			}
			name := m.Name
			if name == "" {
				name = id
			}
			models = append(models, ai.Model{
				ID:              id,
				Name:            name,
				ContextLimit:    m.Limit.Context,
				OutputLimit:     m.Limit.Output,
				InputPrice:      m.Cost.Input,
				OutputPrice:     m.Cost.Output,
				CacheReadPrice:  m.Cost.CacheRead,
				CacheWritePrice: m.Cost.CacheWrite,
				Reasoning:       m.Reasoning,
				Efforts:         effortValues(m),
				Attachment:      m.Attachment,
				Knowledge:       m.Knowledge,
				Released:        m.ReleaseDate,
			})
		}
		if len(models) > 0 {
			out[slug] = models
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// passesFilter enforces the listing policy: tool calling plus a context
// window the agent loop can actually use.
func passesFilter(m catalogModel) bool {
	return m.ToolCall && m.Limit.Context >= MinContextTokens
}

// effortValues extracts the effort levels a model accepts from its
// reasoning_options: the "effort"-typed option carries the values; models
// with only a toggle or a token budget expose no effort list.
func effortValues(m catalogModel) []string {
	for _, opt := range m.ReasoningOptions {
		if opt.Type == "effort" && len(opt.Values) > 0 {
			return append([]string{}, opt.Values...)
		}
	}
	return nil
}

// fetchCatalogLocked downloads the catalog, writes it atomically to the
// cache, and memoizes the parsed copy. Callers must hold mu.
func fetchCatalogLocked(ctx context.Context) bool {
	data := fetchRaw(ctx)
	if data == nil {
		return false
	}
	models := parseCatalog(data)
	if models == nil {
		return false
	}
	cached, cachedAt = models, time.Now()
	writeCache(data)
	return true
}

// fetchRaw performs the HTTP GET and returns the body, or nil on any failure.
func fetchRaw(ctx context.Context) []byte {
	if os.Getenv("AUTOMERGENT_MODELS_DEV_OFFLINE") != "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil || len(body) == 0 {
		return nil
	}
	return body
}

// writeCache persists the raw catalog atomically (temp + rename), creating
// the cache directory as needed.
func writeCache(data []byte) {
	path := cacheFile()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".models-dev-*")
	if err != nil {
		return
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return
	}
	tmp.Close()
	_ = os.Rename(tmp.Name(), path)
}

// CacheInfo describes the cache state for diagnostics (/doctor).
func CacheInfo() (path string, exists bool, age time.Duration, err error) {
	path = cacheFile()
	info, statErr := os.Stat(path)
	if statErr != nil {
		return path, false, 0, statErr
	}
	return path, true, time.Since(info.ModTime()), nil
}

// SnapshotSize reports how many models the embedded fallback carries for a
// provider (0 when the provider has no snapshot).
func SnapshotSize(provider string) int {
	slug, ok := SlugFor(provider)
	if !ok {
		return 0
	}
	return len(snapshotCatalog[slug])
}
