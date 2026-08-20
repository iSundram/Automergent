package debug

import (
	"context"
	"time"

	"github.com/iSundram/Automergent/internal/ai"
)

// DebugProvider wraps an ai.Provider to log requests and responses
type DebugProvider struct {
	provider ai.Provider
	logger   *Logger
}

func NewDebugProvider(provider ai.Provider, logger *Logger) *DebugProvider {
	return &DebugProvider{
		provider: provider,
		logger:   logger,
	}
}

func (d *DebugProvider) Name() string {
	return d.provider.Name()
}

func (d *DebugProvider) Complete(ctx context.Context, req ai.CompletionRequest) (ai.CompletionResponse, error) {
	start := time.Now()

	// Log request
	if err := d.logger.LogRequest(ctx, d.provider.Name(), req); err != nil {
		// Log error but don't fail the request
	}

	// Execute request
	resp, err := d.provider.Complete(ctx, req)
	duration := time.Since(start)

	// Log response or error
	if err != nil {
		d.logger.LogError(ctx, d.provider.Name(), err, req)
		return nil, err
	}

	d.logger.LogResponse(ctx, d.provider.Name(), resp, duration)

	return resp, err
}

func (d *DebugProvider) Models(ctx context.Context) ([]ai.Model, error) {
	return d.provider.Models(ctx)
}

func (d *DebugProvider) TokenCount(messages []ai.Message) (int, error) {
	return d.provider.TokenCount(messages)
}

func (d *DebugProvider) ContextLimit() int {
	return d.provider.ContextLimit()
}