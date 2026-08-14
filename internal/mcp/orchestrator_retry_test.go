package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestServerRecoveryPolicyRespectsRetryCount(t *testing.T) {
	policy := serverRecoveryPolicy(&ServerConfig{RetryCount: 2})

	if d := policy.Decide(1, errors.New("boom")); !d.Retry {
		t.Fatalf("attempt 1 should retry, got %+v", d)
	}
	if d := policy.Decide(2, errors.New("boom")); !d.Retry {
		t.Fatalf("attempt 2 should retry, got %+v", d)
	}
	if d := policy.Decide(3, errors.New("boom")); d.Retry {
		t.Fatalf("attempt 3 should stop retrying, got %+v", d)
	}
}

func TestAddServerWrapsConnectionFailureWithAttempts(t *testing.T) {
	orch := NewOrchestrator(OrchestratorConfig{})
	cfg := &ServerConfig{
		Name:       "bad-server",
		URL:        "http://127.0.0.1:1",
		Transport:  TransportHTTP,
		Enabled:    true,
		RetryCount: 0,
	}

	err := orch.addServer(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected connection error")
	}
	if !strings.Contains(err.Error(), "failed after 1 attempt") {
		t.Fatalf("unexpected error: %v", err)
	}
}
