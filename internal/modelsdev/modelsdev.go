// Package modelsdev loads the community model catalog from models.dev
// (https://models.dev/api.json). The catalog
// gives every model its display name, context/output limits, pricing,
// reasoning capability and knowledge cutoff without a live provider API
// call.
//
// Resolution order:
//
//  1. disk cache (~/.automergent/cache/models-dev.json) while fresh,
//  2. the network (written atomically to that cache),
//  3. the stale disk cache when the network is unreachable,
//  4. the embedded snapshot as the final fallback.
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
	sourceURL   = "https://models.dev/api.json"
	cacheTTL    = 5 * time.Minute
	refreshEvery = time.Hour
)

var (
	mu        sync.Mutex
	cached    []ai.Model
	cachedAt  time.Time
	cachePath string
)

// googleProviderID is the catalog's provider key both Google backends use.
const googleProviderID = "google"

// Models returns the catalog's model list for the google provider (shared
// by google-aistudio and google-vertex). It never fails: every fallback
// ends at the embedded snapshot, so callers get a usable list even fully
// offline on first run.
func Models(ctx context.Context) []ai.Model {
	mu.Lock()
	defer mu.Unlock()
	if cached != nil && time.Since(cachedAt) < cacheTTL {
		return append([]ai.Model{}, cached...)
	}

	models := loadCachedFile()
	if models != nil {
		cached, cachedAt = models, time.Now()
		return append([]ai.Model{}, models...)
	}

	models = fetchCatalog(ctx)
	if models != nil {
		cached, cachedAt = models, time.Now()
		return append([]ai.Model{}, models...)
	}

	// Network unavailable: keep the embedded snapshot. cached stays nil so a
	// later call can retry when connectivity returns.
	return append([]ai.Model{}, snapshotModels...)
}

// Refresh forces a network refresh (backs /model refresh and the hourly
// background refresh). Failure is silent: the existing cache stays valid.
func Refresh(ctx context.Context) {
	models := fetchCatalog(ctx)
	if models == nil {
		return
	}
	mu.Lock()
	cached, cachedAt = models, time.Now()
	mu.Unlock()
}

// StartRefresher runs the periodic background refresh (hourly, like
// until ctx is cancelled.
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
func loadCachedFile() []ai.Model {
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
	Name       string `json:"name"`
	Reasoning  bool   `json:"reasoning"`
	Attachment bool   `json:"attachment"`
	ToolCall   bool   `json:"tool_call"`
	Knowledge  string `json:"knowledge"`
	Limit      struct {
		Context int `json:"context"`
		Output  int `json:"output"`
	} `json:"limit"`
	Cost struct {
		Input  float64 `json:"input"`
		Output float64 `json:"output"`
	} `json:"cost"`
}

// parseCatalog extracts the google provider's tool-call capable models.
func parseCatalog(data []byte) []ai.Model {
	var catalog map[string]catalogProvider
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil
	}
	gp, ok := catalog[googleProviderID]
	if !ok || len(gp.Models) == 0 {
		return nil
	}
	var out []ai.Model
	for id, m := range gp.Models {
		if !m.ToolCall {
			continue // coding agents need tool calling
		}
		name := m.Name
		if name == "" {
			name = id
		}
		out = append(out, ai.Model{
			ID:           id,
			Name:         name,
			ContextLimit: m.Limit.Context,
			OutputLimit:  m.Limit.Output,
			InputPrice:   m.Cost.Input,
			OutputPrice:  m.Cost.Output,
			Reasoning:    m.Reasoning,
			Attachment:   m.Attachment,
			Knowledge:    m.Knowledge,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// fetchCatalog downloads the catalog and writes it atomically to the cache.
func fetchCatalog(ctx context.Context) []ai.Model {
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
	models := parseCatalog(body)
	if models == nil {
		return nil
	}
	writeCache(body)
	return models
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

// SnapshotSize reports how many models the embedded fallback carries.
func SnapshotSize() int { return len(snapshotModels) }
