package prompt

import (
	"context"
	"strings"

	"github.com/iSundram/Automergent/internal/ai"
)

// AIProviderAdapter adapts ai.Provider to LLMClient interface.
type AIProviderAdapter struct {
	provider ai.Provider
	model    string
}

func NewAIProviderAdapter(provider ai.Provider, model string) *AIProviderAdapter {
	if model == "" {
		models, err := provider.Models(context.Background())
		if err == nil && len(models) > 0 {
			model = models[0].ID
		}
	}
	return &AIProviderAdapter{provider: provider, model: model}
}

func (a *AIProviderAdapter) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	req := ai.CompletionRequest{
		Messages: []ai.Message{
			{Role: ai.RoleUser, Content: []ai.ContentPart{{Type: ai.ContentTypeText, Text: userPrompt}}},
		},
		System:      systemPrompt,
		Temperature: 0.1,
		MaxTokens:   4000,
		Stream:      true,
	}

	resp, err := a.provider.Complete(ctx, req)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	stream := resp.Stream()
	for chunk := range stream {
		sb.WriteString(chunk.Text)
		if chunk.Error != nil {
			return sb.String(), chunk.Error
		}
	}

	return strings.TrimSpace(sb.String()), nil
}