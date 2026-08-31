package modelsdev

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// resetCache clears the package memoization so each test starts clean.
func resetCache(t *testing.T, path string) {
	t.Helper()
	mu.Lock()
	cached, cachedAt, cachePath = nil, time.Time{}, path
	mu.Unlock()
	t.Setenv("AUTOMERGENT_MODELS_DEV_OFFLINE", "1")
	if path != "" {
		t.Setenv("AUTOMERGENT_MODELS_DEV_PATH", path)
	}
}

// fixture carries one catalog provider per filter outcome: google has a
// 1M-context tool-call model (listed), a short-context model (filtered),
// and a no-tools model (filtered); anthropic has exactly-1M-context models
// (listed — the threshold is >=, and Anthropic ships 1,000,000 not 2^20).
const fixture = `{
  "google": {
    "models": {
      "gemini-test-pro": {
        "name": "Gemini Test Pro", "reasoning": true, "tool_call": true,
        "attachment": true, "knowledge": "2026-03", "release_date": "2026-05-01",
        "limit": {"context": 1048576, "output": 65536},
        "cost": {"input": 1.25, "output": 10.0, "cache_read": 0.125, "cache_write": 1.875}
      },
      "no-tools-model": {"name": "No Tools", "tool_call": false, "limit": {"context": 1048576}},
      "short-context-model": {"name": "Short", "tool_call": true, "limit": {"context": 524288}},
      "gemini-test-flash": {
        "name": "Gemini Test Flash", "tool_call": true,
        "limit": {"context": 1000000, "output": 16384}
      }
    }
  },
  "anthropic": {
    "models": {
      "claude-test": {
        "name": "Claude Test", "tool_call": true, "reasoning": true,
        "limit": {"context": 1000000, "output": 64000},
        "cost": {"input": 3, "output": 15, "cache_read": 0.3, "cache_write": 3.75}
      }
    }
  }
}`

func TestModelsFromDiskCachePerProvider(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models-dev.json")
	if err := os.WriteFile(path, []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	resetCache(t, path)

	// One cached catalog serves every provider.
	google := Models(context.Background(), "google-aistudio")
	if len(google) != 2 {
		t.Fatalf("expected 2 listed google models, got %d", len(google))
	}
	byID := map[string]int{}
	for i, m := range google {
		byID[m.ID] = i
	}
	pro := google[byID["gemini-test-pro"]]
	if pro.Name != "Gemini Test Pro" || pro.ContextLimit != 1048576 || pro.OutputLimit != 65536 {
		t.Fatalf("pro model metadata wrong: %+v", pro)
	}
	if pro.InputPrice != 1.25 || pro.OutputPrice != 10.0 {
		t.Fatalf("pricing wrong: %+v", pro)
	}
	if pro.CacheReadPrice != 0.125 || pro.CacheWritePrice != 1.875 {
		t.Fatalf("cache pricing wrong: %+v", pro)
	}
	if !pro.Reasoning || !pro.Attachment || pro.Knowledge != "2026-03" || pro.Released != "2026-05-01" {
		t.Fatalf("capability metadata wrong: %+v", pro)
	}
	if _, ok := byID["no-tools-model"]; ok {
		t.Fatal("models without tool calling must be filtered out")
	}
	if _, ok := byID["short-context-model"]; ok {
		t.Fatal("models below MinContextTokens must be filtered out")
	}

	// Legacy and vertex aliases hit the same google slug.
	if got := Models(context.Background(), "google-vertex"); len(got) != len(google) {
		t.Fatalf("vertex alias must match aistudio: %d vs %d", len(got), len(google))
	}
	if got := Models(context.Background(), "google"); len(got) != len(google) {
		t.Fatalf("legacy google alias must match: %d vs %d", len(got), len(google))
	}

	anthropic := Models(context.Background(), "anthropic")
	if len(anthropic) != 1 || anthropic[0].ID != "claude-test" {
		t.Fatalf("anthropic models wrong: %+v", anthropic)
	}
	if anthropic[0].ContextLimit != 1000000 {
		t.Fatal("exactly-1M context must pass the filter (>= threshold)")
	}
}

func TestModelsUnknownProvider(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models-dev.json")
	if err := os.WriteFile(path, []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	resetCache(t, path)
	if models := Models(context.Background(), "custom-endpoint"); models != nil {
		t.Fatalf("providers without a catalog slug must get nil, got %d models", len(models))
	}
	if _, ok := SlugFor("custom-endpoint"); ok {
		t.Fatal("SlugFor must report custom endpoints as unknown")
	}
}

func TestModelInfo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models-dev.json")
	if err := os.WriteFile(path, []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	resetCache(t, path)

	m, ok := ModelInfo(context.Background(), "google", "gemini-test-pro")
	if !ok || m.ID != "gemini-test-pro" || m.Name != "Gemini Test Pro" {
		t.Fatalf("ModelInfo must resolve catalog entries: %+v %v", m, ok)
	}
	if _, ok := ModelInfo(context.Background(), "google", "does-not-exist"); ok {
		t.Fatal("ModelInfo must report unknown models")
	}
}

func TestModelsFallBackToSnapshot(t *testing.T) {
	// No cache file, network disabled: the embedded snapshot answers.
	resetCache(t, filepath.Join(t.TempDir(), "missing.json"))
	for _, provider := range []string{"google", "anthropic", "openai"} {
		if SnapshotSize(provider) == 0 {
			t.Errorf("snapshot for %s must not be empty", provider)
			continue
		}
		models := Models(context.Background(), provider)
		if len(models) != SnapshotSize(provider) {
			t.Errorf("%s: expected the embedded snapshot (%d models), got %d", provider, SnapshotSize(provider), len(models))
		}
		for _, m := range models {
			if m.ContextLimit < MinContextTokens {
				t.Errorf("%s: snapshot model %s below context floor: %d", provider, m.ID, m.ContextLimit)
			}
		}
	}
	if SnapshotSize("custom-endpoint") != 0 {
		t.Fatal("snapshot size for unknown providers must be 0")
	}
	var hasFlash bool
	for _, m := range Models(context.Background(), "google") {
		if strings.Contains(m.ID, "flash") {
			hasFlash = true
		}
	}
	if !hasFlash {
		t.Fatal("snapshot must carry flash models")
	}
}

func TestParseCatalogRejectsGarbage(t *testing.T) {
	if parseCatalog([]byte("not json")) != nil {
		t.Fatal("garbage must not parse")
	}
	if parseCatalog([]byte(`{"other": {}}`)) != nil {
		t.Fatal("a catalog without known providers must not parse")
	}
	// A provider whose models all fail the filter is dropped entirely.
	if parseCatalog([]byte(`{"google": {"models": {"x": {"tool_call": false}}}}`)) != nil {
		t.Fatal("a provider with no listing-eligible models must not parse")
	}
}

func TestCacheInfo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models-dev.json")
	if err := os.WriteFile(path, []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	resetCache(t, path)
	p, exists, _, err := CacheInfo()
	if err != nil || !exists {
		t.Fatalf("CacheInfo must find the cache: %v %v", exists, err)
	}
	if p != path {
		t.Fatalf("CacheInfo path %q, want %q", p, path)
	}
}
