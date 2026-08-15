package context

import (
	"testing"
	"time"
)

func TestContextStateTransitions(t *testing.T) {
	item := ContextItem{
		Path:    "test.go",
		Content: "package main",
		Tokens:  10,
	}

	if item.State != 0 {
		t.Errorf("expected default state to be active, got %d", item.State)
	}

	now := time.Now()
	item.State = ContextIgnored
	item.IgnoredAt = &now
	item.IgnoreReason = "not relevant"

	if item.State != ContextIgnored {
		t.Errorf("expected state to be ignored, got %d", item.State)
	}

	item.State = ContextResumed
	item.ResumedAt = &now
	item.IgnoredAt = nil

	if item.State != ContextResumed {
		t.Errorf("expected state to be resumed, got %d", item.State)
	}
}

func TestPriorityLevels(t *testing.T) {
	tests := []struct {
		priority ContextPriority
		expected string
	}{
		{PriorityCritical, "critical"},
		{PriorityHigh, "high"},
		{PriorityMedium, "medium"},
		{PriorityLow, "low"},
		{PriorityLazy, "lazy"},
	}

	for _, tt := range tests {
		if got := tt.priority.String(); got != tt.expected {
			t.Errorf("PriorityLevel(%d).String() = %s, want %s", tt.priority, got, tt.expected)
		}
	}
}

func TestContextSummarizerExtractive(t *testing.T) {
	s := NewContextSummarizer(nil)

	item := ContextItem{
		Path: "main.go",
		Content: `package main

import "fmt"

// Main is the entry point
func Main() {
	fmt.Println("hello")
}

type Config struct {
	Name string
}
`,
		Tokens: 50,
	}

	summary, tokens, err := s.Summarize(item, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tokens == 0 {
		t.Error("expected non-zero token count")
	}

	if summary == "" {
		t.Error("expected non-empty summary")
	}
}

func TestMemoryRelevantTo(t *testing.T) {
	m := &Memory{
		store: map[string]string{
			"config_path": "/etc/app/config.yaml",
			"user_name":   "john",
			"api_key":     "secret123",
		},
	}

	entries := m.RelevantTo("config path location")
	if len(entries) == 0 {
		t.Error("expected at least one relevant entry")
	}

	found := false
	for _, e := range entries {
		if e.Key == "config_path" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected config_path to be relevant")
	}
}

func TestToolContextPatterns(t *testing.T) {
	tests := []struct {
		tool     string
		expected int
	}{
		{"glob", 1},
		{"grep", 1},
		{"view", 1},
		{"edit", 1},
		{"unknown_tool", 1},
	}

	for _, tt := range tests {
		patterns := toolContextPatterns(tt.tool)
		if len(patterns) < tt.expected {
			t.Errorf("toolContextPatterns(%s) returned %d patterns, want at least %d",
				tt.tool, len(patterns), tt.expected)
		}
	}
}

func TestMatchesAnyPattern(t *testing.T) {
	tests := []struct {
		path     string
		patterns map[string]bool
		expected bool
	}{
		{"src/main.go", map[string]bool{"src": true}, true},
		{"internal/config.go", map[string]bool{"config": true}, true},
		{"test.go", map[string]bool{"go": true}, true},
		{"test.py", map[string]bool{"py": true}, true},
		{"test.txt", map[string]bool{"go": true}, false},
		{"binary.exe", map[string]bool{"readme": true}, false},
		{"anything.txt", map[string]bool{}, true},
	}

	for _, tt := range tests {
		got := matchesAnyPattern(tt.path, tt.patterns)
		if got != tt.expected {
			t.Errorf("matchesAnyPattern(%s, %v) = %v, want %v", tt.path, tt.patterns, got, tt.expected)
		}
	}
}
