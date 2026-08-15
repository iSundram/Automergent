package context

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestContextSelectorIncludesSignals(t *testing.T) {
	dir := t.TempDir()
	active := filepath.Join(dir, "active.go")
	dependent := filepath.Join(dir, "dependent.go")

	mustWrite(t, active, "package main\n")
	mustWrite(t, dependent, "package main\n")

	graph := NewDependencyGraph()
	graph.SetDependencies(dependent, []string{active})
	ranker := NewRanker(DefaultRankingConfig())
	budget := NewTokenBudget(ModelLimitsDefault, DefaultBudgetConfig())
	detector := NewStalenessDetector(dir, StalenessConfig{StaleAfter: time.Hour})
	sel := NewContextSelector(graph, ranker, budget, detector)

	items, signals, err := sel.SelectContext(context.Background(), "inspect active", []string{active}, 1000, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if len(signals) != 2 {
		t.Fatalf("expected 2 signals, got %d", len(signals))
	}
	foundRequired := false
	for _, sig := range signals {
		if sig.Required {
			foundRequired = true
		}
	}
	if !foundRequired {
		t.Fatal("expected active file to be marked required")
	}
}

func TestContextSelectorDependencyToggle(t *testing.T) {
	dir := t.TempDir()
	active := filepath.Join(dir, "active.go")
	dependent := filepath.Join(dir, "dependent.go")

	mustWrite(t, active, "package main\n")
	mustWrite(t, dependent, "package main\n")

	graph := NewDependencyGraph()
	graph.SetDependencies(dependent, []string{active})
	ranker := NewRanker(DefaultRankingConfig())
	budget := NewTokenBudget(ModelLimitsDefault, DefaultBudgetConfig())
	sel := NewContextSelector(graph, ranker, budget, NewStalenessDetector(dir, StalenessConfig{}))

	items, _, err := sel.SelectContext(context.Background(), "inspect active", []string{active}, 1000, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item without dependent traversal, got %d", len(items))
	}
}

func TestStalenessSignalsInfluenceRanking(t *testing.T) {
	ranker := NewRanker(DefaultRankingConfig())
	now := time.Now()
	files := []FileContext{
		{Path: "/fresh.go", Content: "fresh", ModTime: now, Freshness: 1, FreshnessState: "fresh"},
		{Path: "/stale.go", Content: "stale", ModTime: now.Add(-72 * time.Hour), Staleness: 1, FreshnessState: "stale"},
	}

	scores := ranker.RankFiles(files, "fresh", 0)
	if len(scores) != 2 {
		t.Fatalf("expected 2 scores, got %d", len(scores))
	}
	if scores[0].Path != "/fresh.go" {
		t.Fatalf("expected fresh file first, got %s", scores[0].Path)
	}
	if scores[0].FreshnessWeight <= scores[1].FreshnessWeight {
		t.Fatal("expected freshness to increase score")
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
