package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Rule store: standing instructions the user states in conversation ("never
// use tabs", "always write tests first") that INIT captures as rule parts.
// Rules persist per project in .automergent/rules.md and ride in the
// <project-instructions> user message, so they survive sessions and
// compaction. A later "remove that rule" request deletes the line.

const (
	rulesDir      = ".automergent"
	rulesFile     = "rules.md"
	rulesHeader   = "# Project Rules\n\nRules stated by the user. Obey these in all work in this repository.\n"
	maxRuleLength = 500
)

// RuleStore manages the per-project rules file. Safe for concurrent use.
type RuleStore struct {
	mu      sync.Mutex
	rootDir string
}

// NewRuleStore creates a store rooted at the project directory.
func NewRuleStore(rootDir string) *RuleStore {
	return &RuleStore{rootDir: rootDir}
}

func (s *RuleStore) path() string {
	return filepath.Join(s.rootDir, rulesDir, rulesFile)
}

// Add appends a rule. Returns the confirmation line to show the user.
// Duplicate rules (case-insensitive exact match) are idempotent.
func (s *RuleStore) Add(rule string) (string, error) {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return "", fmt.Errorf("empty rule")
	}
	if len(rule) > maxRuleLength {
		rule = rule[:maxRuleLength]
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	existing, _ := os.ReadFile(s.path())
	lines := parseRuleLines(string(existing))
	for _, line := range lines {
		if strings.EqualFold(line, rule) {
			return "rule already recorded: " + rule, nil
		}
	}

	var sb strings.Builder
	sb.WriteString(strings.TrimRight(string(existing), "\n"))
	if len(strings.TrimSpace(string(existing))) == 0 {
		sb.WriteString(rulesHeader)
	} else {
		sb.WriteString("\n")
	}
	sb.WriteString("- " + rule + "\n")

	if err := os.MkdirAll(filepath.Dir(s.path()), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(s.path(), []byte(sb.String()), 0o644); err != nil {
		return "", err
	}
	return "rule recorded: " + rule, nil
}

// Remove deletes a rule by case-insensitive substring match (the user says
// "remove that rule about tabs"). Returns the removed rule and true, or
// false when nothing matched.
func (s *RuleStore) Remove(needle string) (string, bool) {
	needle = strings.ToLower(strings.TrimSpace(needle))
	if needle == "" {
		return "", false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	existing, err := os.ReadFile(s.path())
	if err != nil {
		return "", false
	}
	lines := parseRuleLines(string(existing))

	var kept, removed []string
	for _, line := range lines {
		if needle == strings.ToLower(line) || strings.Contains(strings.ToLower(line), needle) {
			removed = append(removed, line)
			continue
		}
		kept = append(kept, line)
	}
	if len(removed) == 0 {
		return "", false
	}

	var sb strings.Builder
	if len(kept) > 0 {
		sb.WriteString(rulesHeader)
		for _, line := range kept {
			sb.WriteString("- " + line + "\n")
		}
	}
	if err := os.WriteFile(s.path(), []byte(sb.String()), 0o644); err != nil {
		return "", false
	}
	return strings.Join(removed, "; "), true
}

// List returns the recorded rules.
func (s *RuleStore) List() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, err := os.ReadFile(s.path())
	if err != nil {
		return nil
	}
	return parseRuleLines(string(existing))
}

// parseRuleLines extracts the bullet lines from a rules file, ignoring the
// header.
func parseRuleLines(content string) []string {
	var rules []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		rules = append(rules, strings.TrimSpace(strings.TrimPrefix(line, "- ")))
	}
	return rules
}

// ruleStore returns the agent's rule store, rooted at the working directory.
func (a *Agent) ruleStore() *RuleStore {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.rules == nil {
		a.rules = NewRuleStore(a.workDir)
	}
	return a.rules
}
