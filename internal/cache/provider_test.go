package cache

import (
	"context"
	"strings"
	"testing"

	"github.com/iSundram/Automergent/internal/ai"
)

type stubProvider struct {
	calls int
}

func (s *stubProvider) Name() string { return "stub" }

func (s *stubProvider) Complete(_ context.Context, _ ai.CompletionRequest) (ai.CompletionResponse, error) {
	s.calls++
	return ai.NewStaticResponse("ok", "", nil, ai.StopReasonEnd, ai.Usage{}), nil
}

func (s *stubProvider) Models(context.Context) ([]ai.Model, error) { return nil, nil }
func (s *stubProvider) TokenCount([]ai.Message) (int, error)       { return 0, nil }
func (s *stubProvider) ContextLimit() int                          { return 128000 }

func TestCachingProviderRecordsHitAndMiss(t *testing.T) {
	provider := &stubProvider{}
	pc := NewPromptCache()
	cp := NewCachingProvider(provider, pc)

	req := ai.CompletionRequest{
		System:   strings.Repeat("system prompt ", 10),
		Tools:    []ai.ToolSchema{{Name: "echo"}},
		Messages: []ai.Message{ai.NewTextMessage(ai.RoleUser, "hello")},
	}

	if _, err := cp.Complete(context.Background(), req); err != nil {
		t.Fatalf("first completion failed: %v", err)
	}
	if _, err := cp.Complete(context.Background(), req); err != nil {
		t.Fatalf("second completion failed: %v", err)
	}

	stats := pc.Stats()
	if stats.Misses == 0 {
		t.Fatalf("expected at least one cache miss, got %+v", stats)
	}
	if stats.Hits == 0 {
		t.Fatalf("expected at least one cache hit, got %+v", stats)
	}
}

func TestCachingProviderFallsBackWithoutCache(t *testing.T) {
	provider := &stubProvider{}
	cp := NewCachingProvider(provider, nil)

	req := ai.CompletionRequest{
		Messages: []ai.Message{ai.NewTextMessage(ai.RoleUser, "hello")},
	}

	if _, err := cp.Complete(context.Background(), req); err != nil {
		t.Fatalf("completion failed with nil cache: %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("expected provider to be called once, got %d", provider.calls)
	}
}
