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

const fixture = `{
  "google": {
    "models": {
      "gemini-test-pro": {
        "name": "Gemini Test Pro", "reasoning": true, "tool_call": true,
        "attachment": true, "knowledge": "2026-03",
        "limit": {"context": 1048576, "output": 65536},
        "cost": {"input": 1.25, "output": 10.0}
      },
      "no-tools-model": {"name": "No Tools", "tool_call": false},
      "gemini-test-flash": {
        "name": "Gemini Test Flash", "tool_call": true,
        "limit": {"context": 524288, "output": 16384}
      }
    }
  }
}`

func TestModelsFromDiskCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models-dev.json")
	if err := os.WriteFile(path, []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	resetCache(t, path)

	models := Models(context.Background())
	if len(models) != 2 {
		t.Fatalf("expected 2 tool-call models from cache, got %d", len(models))
	}
	byID := map[string]int{}
	for i, m := range models {
		byID[m.ID] = i
	}
	pro := models[byID["gemini-test-pro"]]
	if pro.Name != "Gemini Test Pro" || pro.ContextLimit != 1048576 || pro.OutputLimit != 65536 {
		t.Fatalf("pro model metadata wrong: %+v", pro)
	}
	if pro.InputPrice != 1.25 || pro.OutputPrice != 10.0 {
		t.Fatalf("pricing wrong: %+v", pro)
	}
	if !pro.Reasoning || !pro.Attachment || pro.Knowledge != "2026-03" {
		t.Fatalf("capability metadata wrong: %+v", pro)
	}
	if _, ok := byID["no-tools-model"]; ok {
		t.Fatal("models without tool calling must be filtered out")
	}
}

func TestModelsFallBackToSnapshot(t *testing.T) {
	// No cache file, network disabled: the embedded snapshot answers.
	resetCache(t, filepath.Join(t.TempDir(), "missing.json"))
	if SnapshotSize() == 0 {
		t.Skip("snapshot not generated")
	}
	models := Models(context.Background())
	if len(models) != SnapshotSize() {
		t.Fatalf("expected the embedded snapshot (%d models), got %d", SnapshotSize(), len(models))
	}
	var hasFlash bool
	for _, m := range models {
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
		t.Fatal("a catalog without the google provider must not parse")
	}
	if parseCatalog([]byte(`{"google": {"models": {"x": {"tool_call": false}}}}`)) != nil {
		t.Fatal("a google entry with no tool-call models must not parse")
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
