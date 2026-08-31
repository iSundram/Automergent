package diagnostics

import (
	"container/list"
	"strings"
	"testing"
	"time"
)

// ─── in-memory cache ──────────────────────────────────────────────────────────

func TestCache_RoundTripAndExpiry(t *testing.T) {
	key := cacheKey("/tmp/x.go", "package main")
	diags := []Diagnostic{{Line: 2, Severity: "error", Code: "syntax-error", Message: "boom", Source: "tree-sitter-go"}}

	cachePut(key, diags)
	got, ok := cacheGet(key)
	if !ok || len(got) != 1 || got[0].Message != "boom" {
		t.Fatalf("cache get after put failed: ok=%v got=%v", ok, got)
	}

	// Simulate expiry by backdating the entry; the TTL itself comes from
	// config (default 30s) and is not directly injectable.
	cacheMu.Lock()
	el := cache[key]
	el.Value.(*cacheEntry).expires = time.Now().Add(-time.Second)
	cacheMu.Unlock()

	if _, ok := cacheGet(key); ok {
		t.Fatal("expired entry must be a miss")
	}
}

// cacheGet must return a copy — callers append to and sort results, and
// mutating the cached slice would corrupt future hits.
func TestCache_ReturnsCopy(t *testing.T) {
	key := cacheKey("/tmp/y.go", "package main")
	cachePut(key, []Diagnostic{{Line: 1, Severity: "error", Code: "a", Message: "original", Source: "s"}})

	got, _ := cacheGet(key)
	got[0].Message = "mutated"

	again, _ := cacheGet(key)
	if again[0].Message != "original" {
		t.Fatalf("cache slice was aliased: message mutated to %q", again[0].Message)
	}
}

func TestCache_EvictsAtBound(t *testing.T) {
	savedCache, savedLRU := cache, lru
	cache = make(map[string]*list.Element)
	lru = list.List{}
	defer func() { cache, lru = savedCache, savedLRU }()

	for i := 0; i < maxCacheEntries+50; i++ {
		cachePut(cacheKey("/tmp/f.go", strings.Repeat("x", i+1)), nil)
	}
	if len(cache) > maxCacheEntries {
		t.Fatalf("cache exceeds bound: %d > %d", len(cache), maxCacheEntries)
	}
	// The most recent entry must survive eviction.
	last := cacheKey("/tmp/f.go", strings.Repeat("x", maxCacheEntries+50))
	if _, ok := cacheGet(last); !ok {
		t.Fatal("most recent entry was evicted")
	}
}

// Cache keys must not embed the full content — otherwise every file version
// ever analyzed stays resident in the map.
func TestCache_KeyDoesNotEmbedContent(t *testing.T) {
	content := strings.Repeat("a very long line of file content\n", 1000)
	key := cacheKey("/tmp/big.go", content)
	if strings.Contains(key, "a very long line") {
		t.Fatal("cache key embeds file content")
	}
}

// ─── YAML/TOML duplicate-key nesting ─────────────────────────────────────────

// Same key at different indentation is a different mapping, not a duplicate.
func TestYAML_NestedKeysNotDuplicates(t *testing.T) {
	src := `apiVersion: v1
kind: Deployment
metadata:
  name: app
  labels:
    name: app
spec:
  template:
    metadata:
      name: app
`
	for _, d := range Analyze("deploy.yaml", src) {
		if d.Code == "duplicate-key" {
			t.Fatalf("nested key flagged as duplicate: %+v", d)
		}
	}
}

func TestYAML_SameLevelDuplicateFlagged(t *testing.T) {
	src := "name: first\nvalue: 1\nname: second\n"
	found := false
	for _, d := range Analyze("x.yaml", src) {
		if d.Code == "duplicate-key" && d.Line == 3 {
			found = true
		}
	}
	if !found {
		t.Fatal("genuine same-level duplicate not flagged")
	}
}

func TestTOML_TableScopesNotDuplicates(t *testing.T) {
	src := "[a]\nkey = 1\n\n[b]\nkey = 2\n"
	for _, d := range Analyze("x.toml", src) {
		if d.Code == "duplicate-key" {
			t.Fatalf("same key in different tables flagged: %+v", d)
		}
	}
}

func TestTOML_SameTableDuplicateFlagged(t *testing.T) {
	src := "[a]\nkey = 1\nkey = 2\n"
	found := false
	for _, d := range Analyze("x.toml", src) {
		if d.Code == "duplicate-key" {
			found = true
		}
	}
	if !found {
		t.Fatal("genuine same-table duplicate not flagged")
	}
}

// ─── rule scoping ─────────────────────────────────────────────────────────────

// Headers are not translation units; missing-main must not fire on them.
func TestCHeader_NoMissingMain(t *testing.T) {
	src := "int helper(int x);\n"
	for _, d := range Analyze("util.h", src) {
		if d.Code == "missing-main" {
			t.Fatalf("missing-main fired on a header: %+v", d)
		}
	}
}

// A .c source without main still reports the (info-severity) hint.
func TestCSource_MissingMainReported(t *testing.T) {
	src := "int helper(int x) { return x; }\n"
	found := false
	for _, d := range Analyze("util.c", src) {
		if d.Code == "missing-main" {
			found = true
			if d.Severity == "error" {
				t.Fatalf("missing-main must not be error severity: %+v", d)
			}
		}
	}
	if !found {
		t.Fatal("missing-main not reported for .c source")
	}
}
