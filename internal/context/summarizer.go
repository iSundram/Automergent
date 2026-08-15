package context

import (
	"strings"
)

// ContextSummarizer generates summaries for context items.
type ContextSummarizer struct {
	llm func(prompt string) (string, error)
}

// NewContextSummarizer creates a new summarizer.
func NewContextSummarizer(llm func(string) (string, error)) *ContextSummarizer {
	return &ContextSummarizer{llm: llm}
}

// Summarize generates a summary for a context item.
func (s *ContextSummarizer) Summarize(item ContextItem, maxTokens int) (string, int, error) {
	if s.llm != nil {
		return s.summarizeWithLLM(item, maxTokens)
	}
	return s.summarizeExtractive(item, maxTokens)
}

func (s *ContextSummarizer) summarizeWithLLM(item ContextItem, maxTokens int) (string, int, error) {
	maxChars := maxTokens * 4
	content := item.Content
	if len(content) > maxChars*3 {
		content = content[:maxChars*3]
	}

	prompt := "Summarize the following code file concisely, focusing on key types, functions, and purpose.\n\n" +
		"File: " + item.Path + "\n\n" + content

	summary, err := s.llm(prompt)
	if err != nil {
		return s.summarizeExtractive(item, maxTokens)
	}

	tokens := EstimateTokens(summary)
	return summary, tokens, nil
}

func (s *ContextSummarizer) summarizeExtractive(item ContextItem, maxTokens int) (string, int, error) {
	maxChars := maxTokens * 4

	lines := strings.Split(item.Content, "\n")
	var keyLines []string
	totalChars := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if isKeyLine(trimmed) {
			keyLines = append(keyLines, line)
			totalChars += len(line) + 1
			if totalChars > maxChars {
				break
			}
		}
	}

	summary := strings.Join(keyLines, "\n")
	if summary == "" {
		summary = strings.Join(lines[:min(len(lines), 20)], "\n")
	}

	tokens := EstimateTokens(summary)
	return summary, tokens, nil
}

func isKeyLine(line string) bool {
	if strings.HasPrefix(line, "package ") {
		return true
	}
	if strings.HasPrefix(line, "func ") || strings.HasPrefix(line, "type ") || strings.HasPrefix(line, "var ") || strings.HasPrefix(line, "const ") {
		return true
	}
	if strings.HasPrefix(line, "export ") || strings.HasPrefix(line, "import ") {
		return true
	}
	if strings.HasPrefix(line, "class ") || strings.HasPrefix(line, "def ") {
		return true
	}
	if strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "/*") || strings.HasPrefix(line, "*") {
		return true
	}
	return false
}

// SummarizeIgnored generates a summary for an ignored context item.
func (s *ContextSummarizer) SummarizeIgnored(item ContextItem) string {
	maxTokens := 200
	summary, _, _ := s.Summarize(item, maxTokens)
	return summary
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
