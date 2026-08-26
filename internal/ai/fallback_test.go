package ai

import (
	"context"
	"fmt"
	"testing"

	automergentErrors "github.com/iSundram/Automergent/internal/errors"
)

// mockResponse is a minimal CompletionResponse for testing.
type mockResponse struct {
	text string
}

func (r *mockResponse) Stream() <-chan Chunk        { return nil }
func (r *mockResponse) ToolCalls() []ToolCall       { return nil }
func (r *mockResponse) StopReason() StopReason      { return "" }
func (r *mockResponse) Usage() Usage                { return Usage{} }
func (r *mockResponse) GetMetadata() map[string]any { return nil }
func (r *mockResponse) text_() string               { return r.text }

// mockProvider is a minimal Provider for testing fallback chains.
type mockProvider struct {
	name  string
	model string
	ret   CompletionResponse
	err   error
}

func (m *mockProvider) Name() string                                    { return m.name }
func (m *mockProvider) ContextLimit() int                               { return 128000 }
func (m *mockProvider) Models(_ context.Context) ([]Model, error)       { return nil, nil }
func (m *mockProvider) TokenCount(_ []Message) (int, error)             { return 0, nil }
func (m *mockProvider) Complete(_ context.Context, _ CompletionRequest) (CompletionResponse, error) {
	return m.ret, m.err
}

func TestIsFallbackable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil is not fallbackable", nil, false},
		{"context.Canceled is not fallbackable", context.Canceled, false},
		{"plain error is fallbackable (treated as transport)", fmt.Errorf("network blip"), true},
		{"rate limited is fallbackable", automergentErrors.Wrap(fmt.Errorf("rate limited"), automergentErrors.CodeRateLimited, ""), true},
		{"quota exceeded is fallbackable", automergentErrors.Wrap(fmt.Errorf("quota"), automergentErrors.CodeQuotaExceeded, ""), true},
		{"service unavailable is fallbackable", automergentErrors.Wrap(fmt.Errorf("unavail"), automergentErrors.CodeServiceUnavailable, ""), true},
		{"bad gateway is fallbackable", automergentErrors.Wrap(fmt.Errorf("bad gw"), automergentErrors.CodeBadGateway, ""), true},
		{"gateway timeout is fallbackable", automergentErrors.Wrap(fmt.Errorf("timeout"), automergentErrors.CodeGatewayTimeout, ""), true},
		{"server error is fallbackable", automergentErrors.Wrap(fmt.Errorf("500"), automergentErrors.CodeServerError, ""), true},
		{"connection failed is fallbackable", automergentErrors.Wrap(fmt.Errorf("conn"), automergentErrors.CodeConnectionFailed, ""), true},
		{"connection timeout is fallbackable", automergentErrors.Wrap(fmt.Errorf("conn"), automergentErrors.CodeConnectionTimeout, ""), true},
		{"DNS error is fallbackable", automergentErrors.Wrap(fmt.Errorf("dns"), automergentErrors.CodeDNSError, ""), true},
		{"TLS error is fallbackable", automergentErrors.Wrap(fmt.Errorf("tls"), automergentErrors.CodeTLSError, ""), true},
		{"HTTP error is fallbackable", automergentErrors.Wrap(fmt.Errorf("http"), automergentErrors.CodeHTTPError, ""), true},
		{"provider error is fallbackable", automergentErrors.Wrap(fmt.Errorf("prov"), automergentErrors.CodeProviderError, ""), true},
		{"stream error is fallbackable", automergentErrors.Wrap(fmt.Errorf("stream"), automergentErrors.CodeStreamError, ""), true},
		{"unauthorized is NOT fallbackable", automergentErrors.Wrap(fmt.Errorf("auth"), automergentErrors.CodeUnauthorized, ""), false},
		{"permission denied is NOT fallbackable", automergentErrors.Wrap(fmt.Errorf("perm"), automergentErrors.CodePermissionDenied, ""), false},
		{"context too long is NOT fallbackable", automergentErrors.Wrap(fmt.Errorf("ctx"), automergentErrors.CodeContextTooLong, ""), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsFallbackable(tt.err); got != tt.want {
				t.Errorf("IsFallbackable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFallbackChainPrimarySuccess(t *testing.T) {
	primary := &mockProvider{name: "google", ret: &mockResponse{text: "ok"}}
	chain := NewFallbackChain([]Provider{primary}, []string{"google/gemini-flash"})
	if chain.Name() != "google" {
		t.Errorf("Name() = %q, want %q", chain.Name(), "google")
	}
	if chain.ChainLen() != 1 {
		t.Errorf("ChainLen() = %d, want 1", chain.ChainLen())
	}
	resp, err := chain.Complete(context.Background(), CompletionRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r, ok := resp.(*mockResponse); !ok || r.text != "ok" {
		t.Errorf("response text = %v, want %q", resp, "ok")
	}
}

func TestFallbackChainFallsBackOnTransientError(t *testing.T) {
	failPrimary := &mockProvider{name: "google", err: fmt.Errorf("network timeout")}
	backup := &mockProvider{name: "anthropic", ret: &mockResponse{text: "fallback-ok"}}
	chain := NewFallbackChain([]Provider{failPrimary, backup}, []string{"google/gemini", "anthropic/claude"})
	if chain.ChainLen() != 2 {
		t.Fatalf("ChainLen() = %d, want 2", chain.ChainLen())
	}
	resp, err := chain.Complete(context.Background(), CompletionRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	meta := resp.GetMetadata()
	if meta == nil {
		meta = map[string]any{}
	}
	if servedBy, ok := meta["served_by"]; !ok || servedBy != "anthropic/claude" {
		t.Errorf("served_by = %v, want %q", servedBy, "anthropic/claude")
	}
}

func TestFallbackChainStopsOnDeterministicError(t *testing.T) {
	failPrimary := &mockProvider{name: "google", err: automergentErrors.Wrap(fmt.Errorf("bad key"), automergentErrors.CodeUnauthorized, "invalid API key")}
	backup := &mockProvider{name: "anthropic", ret: &mockResponse{text: "should not reach"}}
	chain := NewFallbackChain([]Provider{failPrimary, backup}, []string{"google/gemini", "anthropic/claude"})
	_, err := chain.Complete(context.Background(), CompletionRequest{})
	if err == nil {
		t.Fatal("expected error from deterministic failure, got nil")
	}
}

func TestFallbackChainExhaustion(t *testing.T) {
	fail1 := &mockProvider{name: "google", err: fmt.Errorf("rate limited")}
	fail2 := &mockProvider{name: "anthropic", err: fmt.Errorf("still broken")}
	chain := NewFallbackChain([]Provider{fail1, fail2}, []string{"google/gemini", "anthropic/claude"})
	_, err := chain.Complete(context.Background(), CompletionRequest{})
	if err == nil {
		t.Fatal("expected error when chain exhausted, got nil")
	}
}

func TestFallbackChainObserverNotification(t *testing.T) {
	var observed bool
	failPrimary := &mockProvider{name: "google", err: fmt.Errorf("connection lost")}
	backup := &mockProvider{name: "anthropic", ret: &mockResponse{text: "ok"}}
	chain := NewFallbackChain([]Provider{failPrimary, backup}, []string{"google/gemini", "anthropic/claude"})
	chain.SetRetryObserver(func(info RetryInfo) {
		observed = true
		if info.Code != "PROVIDER_FALLBACK" {
			t.Errorf("observer code = %q, want PROVIDER_FALLBACK", info.Code)
		}
	})
	chain.Complete(context.Background(), CompletionRequest{})
	if !observed {
		t.Error("observer was not called")
	}
}

func TestFallbackChainLabels(t *testing.T) {
	chain := NewFallbackChain(
		[]Provider{&mockProvider{name: "a"}, &mockProvider{name: "b"}},
		[]string{"a/m1"},
	)
	labels := chain.Labels()
	if len(labels) != 2 {
		t.Fatalf("Labels() len = %d, want 2", len(labels))
	}
	if labels[0] != "a/m1" {
		t.Errorf("Labels()[0] = %q, want %q", labels[0], "a/m1")
	}
	if labels[1] != "b" {
		t.Errorf("Labels()[1] = %q, want %q (fallback to name)", labels[1], "b")
	}
}

func TestFallbackChainFallbackTaggedMetadata(t *testing.T) {
	failPrimary := &mockProvider{name: "google", err: fmt.Errorf("down")}
	backup := &mockProvider{name: "anthropic", ret: &mockResponse{text: "ok"}}
	chain := NewFallbackChain([]Provider{failPrimary, backup}, []string{"google/gemini", "anthropic/claude"})
	resp, _ := chain.Complete(context.Background(), CompletionRequest{})
	meta := resp.GetMetadata()
	if meta == nil {
		t.Fatal("expected metadata, got nil")
	}
	if servedBy, ok := meta["served_by"]; !ok || servedBy != "anthropic/claude" {
		t.Errorf("served_by = %v, want %q", servedBy, "anthropic/claude")
	}
}
