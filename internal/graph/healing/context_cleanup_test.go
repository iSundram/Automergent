package healing

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestGetActivePromptsExcludesExpired(t *testing.T) {
	bucketID := uuid.New()
	cleanup := NewContextCleanup(nil, DefaultStalenessConfig())
	cleanup.injectedPrompts[bucketID] = []*InjectedPrompt{
		{ID: uuid.New(), BucketID: bucketID, Prompt: "expired", ExpiresAt: time.Now().Add(-time.Minute)},
		{ID: uuid.New(), BucketID: bucketID, Prompt: "active", ExpiresAt: time.Now().Add(time.Minute)},
	}

	active := cleanup.GetActivePrompts(bucketID)
	if len(active) != 1 || active[0].Prompt != "active" {
		t.Fatalf("active prompts = %+v, want only active", active)
	}
	stats := cleanup.GetContextStats(bucketID)
	if stats["expired_prompts"] != 1 || stats["active_prompts"] != 1 {
		t.Fatalf("stats = %+v", stats)
	}
}
